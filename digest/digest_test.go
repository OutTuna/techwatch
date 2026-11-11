package digest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/feeds"
)

func TestGenerateAndCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var gotSys, gotUser string
	chat = func(_ context.Context, _ config.Provider, sys, user string) (string, error) {
		gotSys, gotUser = sys, user
		return "**Огляд** дня\n• CVE-2026-1", nil
	}
	items := []feeds.Item{
		{Title: "Zero-day in X", Source: "A", Level: feeds.Critical, CVEs: []string{"CVE-2026-1"}},
		{Title: "o/r v1", Category: "releases", Level: feeds.Low},
	}
	d, err := Generate(context.Background(), config.Provider{Name: "groq"}, "uk", items)
	if err != nil || d.Text != "Огляд дня\n• CVE-2026-1" {
		t.Fatal(d, err)
	}
	if !strings.Contains(gotSys, "Ukrainian") || !strings.Contains(gotSys, "cybersecurity analyst") ||
		!strings.Contains(gotUser, "[CRITICAL] Zero-day in X (A) CVE-2026-1") || strings.Contains(gotUser, "o/r") {
		t.Fatal(gotSys, gotUser)
	}

	if err := Save(d); err != nil {
		t.Fatal(err)
	}
	if Load("uk", "groq", time.Hour) == nil {
		t.Fatal("cache miss")
	}
	if Load("en", "groq", time.Hour) != nil || Load("uk", "gemini", time.Hour) != nil || Load("uk", "groq", -time.Second) != nil {
		t.Fatal("cache must check lang, provider and age")
	}

	if _, err := Generate(context.Background(), config.Provider{}, "uk", items[1:]); err == nil {
		t.Fatal("no news must be an error")
	}
}
