package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/i18n"
	"github.com/OutTuna/techwatch/llm"
)

const batchSize = 30

type Translator struct {
	Cache    *Cache
	Mode     string           // auto | google | llm | none
	Provider *config.Provider // nil = no LLM

	chat   func(ctx context.Context, p config.Provider, system, user string) (string, error)
	google func(ctx context.Context, titles []string, lang string) map[string]string
}

func New(cfg *config.Config) *Translator {
	return &Translator{
		Cache:    OpenCache(filepath.Join(config.Dir(), "translations.json"), cfg.CacheMaxEntries),
		Mode:     cfg.TranslationProvider,
		Provider: cfg.Active(),
		chat:     llm.Chat,
		google:   googleMany,
	}
}

func needs(it feeds.Item, lang string) bool {
	return it.Lang != lang && it.Category != "releases"
}

// Translate fills Translated for items not in lang.
func (t *Translator) Translate(ctx context.Context, items []feeds.Item, lang string) []feeds.Item {
	out := make([]feeds.Item, len(items))
	copy(out, items)
	var titles []string
	var idx []int
	for i := range out {
		out[i].Translated = ""
		if needs(out[i], lang) {
			titles = append(titles, out[i].Title)
			idx = append(idx, i)
		}
	}
	for j, tr := range t.Texts(ctx, titles, lang) {
		out[idx[j]].Translated = tr
	}
	return out
}

// Texts translates headlines or paragraphs; "" where it failed.
// Cache first, only new texts go out. Chain: LLM -> Google -> original.
func (t *Translator) Texts(ctx context.Context, texts []string, lang string) []string {
	res := make([]string, len(texts))
	if t.Mode == "none" {
		return res
	}

	var todo []string
	seen := map[string]bool{}
	for _, s := range texts {
		if seen[s] {
			continue
		}
		seen[s] = true
		if _, ok := t.Cache.Get(lang, s); !ok {
			todo = append(todo, s)
		}
	}

	if len(todo) > 0 {
		var rest []string
		useLLM := t.Provider != nil && (t.Mode == "auto" || t.Mode == "llm")
		for i := 0; i < len(todo); i += batchSize {
			batch := todo[i:min(i+batchSize, len(todo))]
			if useLLM {
				if tr, err := t.llmBatch(ctx, batch, lang); err == nil {
					for j, s := range batch {
						t.Cache.Put(lang, s, tr[j])
					}
					continue
				}
				useLLM = false // 429 or broken answer: don't retry LLM in this run
			}
			rest = append(rest, batch...)
		}
		if len(rest) > 0 && ctx.Err() == nil {
			for s, tr := range t.google(ctx, rest, lang) {
				t.Cache.Put(lang, s, tr)
			}
		}
		_ = t.Cache.Save()
	}

	for i, s := range texts {
		res[i], _ = t.Cache.Get(lang, s)
	}
	return res
}

func (t *Translator) llmBatch(ctx context.Context, batch []string, lang string) ([]string, error) {
	in, _ := json.Marshal(batch)
	system := fmt.Sprintf("You translate tech and security news headlines and article paragraphs into %s. "+
		"Keep CVE IDs, product names, versions and code unchanged. "+
		"Reply with ONLY a JSON array of %d strings, same order, nothing else.",
		i18n.T(lang, "lang.prompt_name"), len(batch))
	ans, err := t.chat(ctx, *t.Provider, system, string(in))
	if err != nil {
		return nil, err
	}
	return parseArray(ans, len(batch))
}

// parseArray tolerates code fences or text around the array.
func parseArray(s string, n int) ([]string, error) {
	a, b := strings.Index(s, "["), strings.LastIndex(s, "]")
	if a < 0 || b < a {
		return nil, fmt.Errorf("no JSON array")
	}
	var res []string
	if err := json.Unmarshal([]byte(s[a:b+1]), &res); err != nil {
		return nil, err
	}
	if len(res) != n {
		return nil, fmt.Errorf("got %d items, want %d", len(res), n)
	}
	for _, r := range res {
		if strings.TrimSpace(r) == "" {
			return nil, fmt.Errorf("empty item")
		}
	}
	return res, nil
}
