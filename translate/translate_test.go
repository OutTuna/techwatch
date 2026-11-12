package translate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/llm"
)

func TestCache(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	c := OpenCache(p, 2)
	c.Put("uk", "a", "А")
	c.Put("uk", "b", "Б")
	c.Put("uk", "c", "В")
	c.m[key("uk", "old")] = entry{"x", time.Now().Add(-8 * 24 * time.Hour)}
	if _, ok := c.Get("uk", "old"); ok {
		t.Fatal("expired entry returned")
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2 := OpenCache(p, 2)
	if len(c2.m) != 2 {
		t.Fatalf("want 2 entries, got %d", len(c2.m))
	}
	if s, ok := c2.Get("uk", "c"); !ok || s != "В" {
		t.Fatal("newest entry lost")
	}
	if _, ok := c2.Get("en", "c"); ok {
		t.Fatal("lang must be part of key")
	}
	os.WriteFile(p, []byte("{broken"), 0o600)
	if len(OpenCache(p, 2).m) != 0 {
		t.Fatal("broken file must give empty cache")
	}
}

func TestParseGoogle(t *testing.T) {
	s, err := parseGoogle([]byte(`[[["Привіт, ","Hello, ",null],["світ","world",null]],null,"en"]`))
	if err != nil || s != "Привіт, світ" {
		t.Fatal(s, err)
	}
	if _, err := parseGoogle([]byte(`[]`)); err == nil {
		t.Fatal("want error")
	}
}

func fakeGoogle(t *testing.T, calls *int, joinWorks bool) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		q := r.URL.Query().Get("q")
		if !joinWorks && strings.Contains(q, "\n") {
			q = strings.ReplaceAll(q, "\n", " ")
		}
		res, _ := json.Marshal([]any{[]any{[]any{"UK:" + q, q}}})
		w.Write(res)
	}))
	t.Cleanup(srv.Close)
	old, oldEvery := googleURL, googleLimit.every
	googleURL, googleLimit.every = srv.URL, 0
	t.Cleanup(func() { googleURL, googleLimit.every = old, oldEvery })
}

func TestGoogleMany(t *testing.T) {
	ctx := context.Background()
	calls := 0
	fakeGoogle(t, &calls, true)
	res := googleMany(ctx, []string{"a", "b", "c"}, "uk")
	if calls != 1 || res["a"] != "UK:a" || !strings.HasSuffix(res["c"], "c") {
		t.Fatal(calls, res)
	}

	calls = 0
	fakeGoogle(t, &calls, false) // service merged lines: fall back to one by one
	res = googleMany(ctx, []string{"a", "b"}, "uk")
	if calls != 3 || res["a"] != "UK:a" || res["b"] != "UK:b" {
		t.Fatal(calls, res)
	}
}

func TestChunks(t *testing.T) {
	c := chunks([]string{"aaaa", "bbbb", "cc"}, 9)
	if len(c) != 2 || len(c[0]) != 2 {
		t.Fatal(c)
	}
}

func newT(t *testing.T, mode string, prov *config.Provider) (*Translator, *[]int, *int) {
	llmCalls, googleCalls := []int{}, 0
	tr := &Translator{
		Cache:    OpenCache(filepath.Join(t.TempDir(), "c.json"), 100),
		Mode:     mode,
		Provider: prov,
		google: func(_ context.Context, titles []string, _ string) map[string]string {
			googleCalls += len(titles)
			out := map[string]string{}
			for _, s := range titles {
				out[s] = "G:" + s
			}
			return out
		},
	}
	return tr, &llmCalls, &googleCalls
}

func items(titles ...string) []feeds.Item {
	var out []feeds.Item
	for _, s := range titles {
		out = append(out, feeds.Item{Title: s, Lang: "en", Category: "security"})
	}
	return out
}

func TestTranslateLLM(t *testing.T) {
	tr, llmCalls, g := newT(t, "auto", &config.Provider{Name: "x"})
	tr.chat = func(_ context.Context, _ config.Provider, _, user string) (string, error) {
		var in []string
		json.Unmarshal([]byte(user), &in)
		*llmCalls = append(*llmCalls, len(in))
		for i := range in {
			in[i] = "L:" + in[i]
		}
		b, _ := json.Marshal(in)
		return "```json\n" + string(b) + "\n```", nil
	}
	var titles []string
	for i := range 45 {
		titles = append(titles, "t"+string(rune('A'+i)))
	}
	src := items(titles...)
	src = append(src, feeds.Item{Title: "Уже українською", Lang: "uk"}, feeds.Item{Title: "o/r v1.0", Lang: "en", Category: "releases"})

	out := tr.Translate(context.Background(), src, "uk")
	if len(*llmCalls) != 2 || (*llmCalls)[0] != 30 || (*llmCalls)[1] != 15 || *g != 0 {
		t.Fatal(*llmCalls, *g)
	}
	if out[0].Translated != "L:tA" || out[45].Translated != "" || out[46].Translated != "" {
		t.Fatal(out[0], out[45], out[46])
	}
	if src[0].Translated != "" {
		t.Fatal("input must not be modified")
	}

	// second run: everything cached, no calls
	tr.Translate(context.Background(), src, "uk")
	if len(*llmCalls) != 2 {
		t.Fatal("cached titles sent again")
	}
}

func TestTranslateFallback(t *testing.T) {
	tr, llmCalls, g := newT(t, "auto", &config.Provider{Name: "x"})
	tr.chat = func(context.Context, config.Provider, string, string) (string, error) {
		*llmCalls = append(*llmCalls, 1)
		return "", llm.ErrRateLimit
	}
	var titles []string
	for i := range 40 {
		titles = append(titles, "t"+string(rune('A'+i)))
	}
	out := tr.Translate(context.Background(), items(titles...), "uk")
	if len(*llmCalls) != 1 || *g != 40 || out[39].Translated != "G:t"+string(rune('A'+39)) {
		t.Fatal(*llmCalls, *g, out[39])
	}

	// wrong item count is also a failure
	tr, _, g = newT(t, "llm", &config.Provider{Name: "x"})
	tr.chat = func(context.Context, config.Provider, string, string) (string, error) { return `["only one"]`, nil }
	tr.Translate(context.Background(), items("a", "b"), "uk")
	if *g != 2 {
		t.Fatal("expected google fallback")
	}

	// google failed too: original stays
	tr, _, _ = newT(t, "google", nil)
	tr.google = func(context.Context, []string, string) map[string]string { return nil }
	if out := tr.Translate(context.Background(), items("a"), "uk"); out[0].Translated != "" {
		t.Fatal(out[0])
	}
}

func TestModes(t *testing.T) {
	tr, _, g := newT(t, "none", &config.Provider{Name: "x"})
	tr.Translate(context.Background(), items("a"), "uk")
	if *g != 0 {
		t.Fatal("none must not translate")
	}
	// google mode ignores provider
	tr, llmCalls, g := newT(t, "google", &config.Provider{Name: "x"})
	tr.chat = func(context.Context, config.Provider, string, string) (string, error) {
		*llmCalls = append(*llmCalls, 1)
		return "", nil
	}
	tr.Translate(context.Background(), items("a"), "uk")
	if *g != 1 || len(*llmCalls) != 0 {
		t.Fatal(*g, *llmCalls)
	}
}

func TestParseArray(t *testing.T) {
	if _, err := parseArray(`["a",""]`, 2); err == nil {
		t.Fatal("empty item must fail")
	}
	if r, err := parseArray(`Here: ["a","b"] done`, 2); err != nil || r[1] != "b" {
		t.Fatal(r, err)
	}
}

// TECHWATCH_LIVE=1 go test ./translate -run Live -v
func TestLiveGoogle(t *testing.T) {
	if os.Getenv("TECHWATCH_LIVE") == "" {
		t.Skip("set TECHWATCH_LIVE=1")
	}
	res := googleMany(context.Background(), []string{"Chrome zero-day actively exploited in the wild", "Microsoft fixes 60 bugs in Patch Tuesday"}, "uk")
	t.Log(res)
	if len(res) != 2 {
		t.Fatal(res)
	}
}

func TestTexts(t *testing.T) {
	tr, _, g := newT(t, "google", nil)
	res := tr.Texts(context.Background(), []string{"p1", "p2", "p1"}, "uk")
	if *g != 2 || res[0] != "G:p1" || res[2] != "G:p1" {
		t.Fatal(*g, res)
	}
	tr.google = func(context.Context, []string, string) map[string]string { return nil }
	if res := tr.Texts(context.Background(), []string{"new"}, "uk"); res[0] != "" {
		t.Fatal("failed text must stay empty")
	}
}
