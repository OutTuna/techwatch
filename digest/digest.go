package digest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/i18n"
	"github.com/OutTuna/techwatch/llm"
)

const maxHeadlines = 60

type Digest struct {
	Text     string    `json:"text"`
	Lang     string    `json:"lang"`
	Provider string    `json:"provider"`
	At       time.Time `json:"at"`
}

var chat = llm.Chat

var ErrNoNews = errors.New("no news yet")

func Generate(ctx context.Context, p config.Provider, lang string, items []feeds.Item) (*Digest, error) {
	var b strings.Builder
	n := 0
	for _, it := range items { // items come sorted by level, so the important ones go first
		if it.Category == "releases" || n == maxHeadlines {
			continue
		}
		n++
		fmt.Fprintf(&b, "[%s] %s (%s)", it.Level, it.Title, it.Source)
		if len(it.CVEs) > 0 {
			b.WriteString(" " + strings.Join(it.CVEs, ","))
		}
		b.WriteString("\n")
	}
	if n == 0 {
		return nil, ErrNoNews
	}
	system := fmt.Sprintf(`You are a cybersecurity analyst writing a short daily brief for engineers.
Use only the headlines given. Write in %s. Plain text, no markdown, at most 14 lines:
first 2-3 sentences with the overall picture, then up to 5 lines starting with "• " about the most important threats (mention CVE IDs and products),
then one line starting with "→ " with what to patch or check first.`, i18n.T(lang, "lang.prompt_name"))
	text, err := chat(ctx, p, system, b.String())
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(strings.NewReplacer("**", "", "##", "").Replace(text))
	if text == "" {
		return nil, fmt.Errorf("empty digest")
	}
	return &Digest{Text: text, Lang: lang, Provider: p.Name, At: time.Now()}, nil
}

func cachePath() string { return filepath.Join(config.Dir(), "digest.json") }

// Load returns the cached digest if it matches lang and provider and is fresh; otherwise nil.
func Load(lang, provider string, maxAge time.Duration) *Digest {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return nil
	}
	var d Digest
	if json.Unmarshal(data, &d) != nil || d.Lang != lang || d.Provider != provider || time.Since(d.At) > maxAge {
		return nil
	}
	return &d
}

func Save(d *Digest) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return config.WriteFile(cachePath(), data)
}
