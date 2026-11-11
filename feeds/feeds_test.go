package feeds

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/OutTuna/techwatch/config"
)

func TestClassify(t *testing.T) {
	cases := map[string]Level{
		"Chrome zero-day actively exploited":     Critical,
		"Fortinet fixes critical RCE in FortiOS": Critical,
		"Flaw rated CVSS 9.8 in Cisco IOS":       Critical,
		"Нова атака на ланцюг постачання npm":    Critical,
		"Віддалене виконання коду в Exim":        Critical,
		"Hospital hit by ransomware":             High,
		"Витік даних клієнтів банку":             High,
		"Pixel 11 leak shows new camera":         Info,
		"Microsoft Patch Tuesday fixes 60 bugs":  Medium,
		"Знайдено вразливість у роутерах":        Medium,
		"Firefox 140 beta":                       Low,
		"Вийшло оновлення Android":               Low,
		"Open source licensing debate":           Info,
		"Force of the source":                    Info, // no "rce" inside words
		"CVSS 1.0 note":                          Low,
	}
	for title, want := range cases {
		if got := Classify(title); got != want {
			t.Errorf("%q: got %v want %v", title, got, want)
		}
	}
}

func TestExtractCVEs(t *testing.T) {
	got := ExtractCVEs("Fix for cve-2026-12345 and CVE-2025-1234", "also CVE-2026-12345")
	if !slices.Equal(got, []string{"CVE-2026-12345", "CVE-2025-1234"}) {
		t.Fatal(got)
	}
}

func TestParseKEV(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	body := []byte(`{"vulnerabilities":[
		{"cveID":"CVE-2026-1111","vulnerabilityName":"Old one","dateAdded":"2025-01-01"},
		{"cveID":"CVE-2026-2222","vulnerabilityName":"Fresh one","dateAdded":"2026-11-10"}]}`)
	items, err := parseKEV(body, now)
	if err != nil || len(items) != 1 || items[0].Level != Critical || items[0].CVEs[0] != "CVE-2026-2222" {
		t.Fatalf("%+v %v", items, err)
	}
}

func TestParseGHSA(t *testing.T) {
	body := []byte(`[{"cve_id":"CVE-2026-3333","html_url":"u","summary":"Path traversal","severity":"high","published_at":"2026-11-11T00:00:00Z"},
		{"cve_id":null,"html_url":"u2","summary":"RCE in parser","severity":"low","published_at":"2026-11-11T00:00:00Z"}]`)
	items, err := parseGHSA(body)
	if err != nil || items[0].Level != High || items[0].CVEs[0] != "CVE-2026-3333" || items[1].Level != Critical || len(items[1].CVEs) != 0 {
		t.Fatalf("%+v %v", items, err)
	}
}

const rss = `<?xml version="1.0"?><rss version="2.0"><channel><title>x</title>
<item><title>New zero-day CVE-2026-4444</title><link>https://a/1</link><pubDate>Wed, 11 Nov 2026 10:00:00 GMT</pubDate></item>
<item><title>Ancient news</title><link>https://a/2</link><pubDate>Mon, 01 Jan 2024 10:00:00 GMT</pubDate></item>
</channel></rss>`

func TestParseFeed(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	items, err := parseFeed([]byte(rss), config.Feed{Name: "S", Category: "security", Lang: "en"}, now)
	if err != nil || len(items) != 1 || items[0].Level != Critical || items[0].CVEs[0] != "CVE-2026-4444" {
		t.Fatalf("%+v %v", items, err)
	}
	// releases ignore age, prefix repo, at least LOW
	items, _ = parseFeed([]byte(rss), config.Feed{Name: "o/r", Category: "releases"}, now)
	if len(items) != 2 || items[1].Title != "o/r Ancient news" || items[1].Level != Low {
		t.Fatalf("%+v", items)
	}
}

func TestDedup(t *testing.T) {
	now := time.Now()
	items := Dedup([]Item{
		{Title: "Same story!", URL: "a", Level: Low, Published: now},
		{Title: "same story", URL: "b", Level: High, Published: now},
		{Title: "Other", URL: "b", Level: Info, Published: now},
		{Title: "Third", URL: "c", Level: Info, Published: now.Add(time.Hour)},
	})
	if len(items) != 2 || items[0].URL != "b" || items[1].Title != "Third" {
		t.Fatalf("%+v", items)
	}
	items = Dedup([]Item{
		{Title: "MSRC one", URL: "m", Category: "vulns", CVEs: []string{"CVE-1"}, Level: Critical},
		{Title: "GHSA one", URL: "g", Category: "vulns", CVEs: []string{"CVE-1"}, Level: High},
		{Title: "News one", URL: "n", Category: "security", CVEs: []string{"CVE-1"}},
	})
	if len(items) != 2 || items[0].Title != "MSRC one" || items[1].Title != "News one" {
		t.Fatalf("%+v", items)
	}
}

func TestSources(t *testing.T) {
	cfg := config.Default()
	cfg.Releases = []string{"nodejs/node", "bad repo/../x"}
	cfg.ExtraFeeds = []config.Feed{{Name: "X", URL: "https://x/feed", Category: "ai"}}
	cfg.DisabledCategories = []string{"local"}
	var rel, local int
	var extra config.Feed
	for _, s := range Sources(cfg) {
		switch {
		case s.Category == "releases":
			rel++
		case s.Category == "local":
			local++
		case s.Name == "X":
			extra = s
		}
	}
	if rel != 1 || local != 0 || extra.Lang != "en" || extra.Category != "tech" {
		t.Fatal(rel, local, extra)
	}
}

// TECHWATCH_LIVE=1 go test ./feeds -run Live -v
func TestLive(t *testing.T) {
	if os.Getenv("TECHWATCH_LIVE") == "" {
		t.Skip("set TECHWATCH_LIVE=1")
	}
	srcs := Sources(config.Default())
	items, failed := Fetch(context.Background(), srcs)
	count := map[string]int{}
	for _, it := range items {
		count[it.Source]++
	}
	for _, s := range srcs {
		t.Logf("%-20s %d", s.Name, count[s.Name])
	}
	if len(failed) > 0 {
		t.Errorf("failed: %v", failed)
	}
}
