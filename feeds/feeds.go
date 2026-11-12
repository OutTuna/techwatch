package feeds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/OutTuna/techwatch/config"
	"github.com/mmcdole/gofeed"
)

type Item struct {
	Title      string
	Translated string
	Summary    string // feed description/content, html; article fallback
	Source     string
	URL        string
	Category   string
	Lang       string
	Published  time.Time
	Level      Level
	CVEs       []string
}

const (
	newsMaxAge  = 48 * time.Hour
	kevMaxAge   = 30 * 24 * time.Hour
	perFeed     = 40
	perRelease  = 3
	maxBodySize = 20 << 20
)

var client = &http.Client{Timeout: 20 * time.Second}

// Fetch loads all sources in parallel. A failing source is reported, not fatal.
func Fetch(ctx context.Context, srcs []config.Feed) (items []Item, failed []string) {
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	now := time.Now()
	for _, src := range srcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := fetchOne(ctx, src, now)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, src.Name)
				return
			}
			items = append(items, got...)
		}()
	}
	wg.Wait()
	sort.Strings(failed)
	return Dedup(items), failed
}

func fetchOne(ctx context.Context, src config.Feed, now time.Time) ([]Item, error) {
	body, err := get(ctx, src.URL)
	if err != nil {
		return nil, err
	}
	switch src.URL {
	case KEVURL:
		return parseKEV(body, now)
	case GHSAURL:
		return parseGHSA(body)
	}
	return parseFeed(body, src, now)
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "techwatch/0.1 (+https://github.com/OutTuna/techwatch)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
}

func parseFeed(body []byte, src config.Feed, now time.Time) ([]Item, error) {
	feed, err := gofeed.NewParser().Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	limit := perFeed
	if src.Category == "releases" {
		limit = perRelease
	}
	var out []Item
	for _, e := range feed.Items {
		if len(out) == limit {
			break
		}
		title := strings.TrimSpace(e.Title)
		if title == "" {
			continue
		}
		pub := now
		if e.PublishedParsed != nil {
			pub = *e.PublishedParsed
		} else if e.UpdatedParsed != nil {
			pub = *e.UpdatedParsed
		}
		if src.Category != "releases" && now.Sub(pub) > newsMaxAge {
			continue
		}
		it := Item{
			Title: title, Source: src.Name, URL: e.Link, Category: src.Category, Lang: src.Lang,
			Published: pub, Level: Classify(title), CVEs: ExtractCVEs(title, e.Description),
			Summary: e.Description,
		}
		if len(e.Content) > len(e.Description) {
			it.Summary = e.Content
		}
		if src.Category == "releases" {
			it.Title = src.Name + " " + title
			it.Level = max(it.Level, Low)
		}
		out = append(out, it)
	}
	return out, nil
}

func parseKEV(body []byte, now time.Time) ([]Item, error) {
	var d struct {
		Vulnerabilities []struct {
			CVE       string `json:"cveID"`
			Name      string `json:"vulnerabilityName"`
			DateAdded string `json:"dateAdded"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	var out []Item
	for _, v := range d.Vulnerabilities {
		added, err := time.Parse("2006-01-02", v.DateAdded)
		if err != nil || now.Sub(added) > kevMaxAge {
			continue
		}
		out = append(out, Item{
			Title: v.Name, Source: "CISA KEV", URL: "https://nvd.nist.gov/vuln/detail/" + v.CVE,
			Category: "vulns", Lang: "en", Published: added, Level: Critical, CVEs: []string{v.CVE},
		})
	}
	return out, nil
}

var ghsaLevels = map[string]Level{"critical": Critical, "high": High, "medium": Medium, "low": Low}

func parseGHSA(body []byte) ([]Item, error) {
	var d []struct {
		CVE       string    `json:"cve_id"`
		URL       string    `json:"html_url"`
		Summary   string    `json:"summary"`
		Severity  string    `json:"severity"`
		Published time.Time `json:"published_at"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(d))
	for _, a := range d {
		out = append(out, Item{
			Title: a.Summary, Source: "GitHub Advisories", URL: a.URL, Category: "vulns", Lang: "en",
			Published: a.Published, Level: max(ghsaLevels[a.Severity], Classify(a.Summary)), CVEs: ExtractCVEs(a.CVE, a.Summary),
		})
	}
	return out, nil
}

// Dedup sorts by level then time and drops repeats by URL, normalized title or single CVE.
func Dedup(items []Item) []Item {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Level != items[j].Level {
			return items[i].Level > items[j].Level
		}
		return items[i].Published.After(items[j].Published)
	})
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		k := norm(it.Title)
		cve := ""
		if it.Category == "vulns" && len(it.CVEs) == 1 {
			cve = it.CVEs[0] // MSRC, GHSA and KEV often list the same CVE
		}
		if seen[k] || it.URL != "" && seen[it.URL] || cve != "" && seen[cve] {
			continue
		}
		seen[k], seen[it.URL], seen[cve] = true, true, true
		out = append(out, it)
	}
	return out
}

func norm(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
