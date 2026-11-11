package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OutTuna/techwatch/config"
)

func TestChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/chat/completions" && r.Header.Get("Authorization") == "Bearer k":
			w.Write([]byte(`{"choices":[{"message":{"content":"hi-openai"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/models/m:generateContent") && r.Header.Get("x-goog-api-key") == "k":
			w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi-gemini"}]}}]}`))
		case r.URL.Path == "/limited/chat/completions":
			w.WriteHeader(429)
		default:
			w.WriteHeader(401)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if s, err := Chat(ctx, config.Provider{Type: "openai", BaseURL: srv.URL, Model: "m", APIKey: "k"}, "s", "u"); s != "hi-openai" {
		t.Fatal(s, err)
	}
	if s, err := Chat(ctx, config.Provider{Type: "gemini", BaseURL: srv.URL, Model: "m", APIKey: "k"}, "s", "u"); s != "hi-gemini" {
		t.Fatal(s, err)
	}
	if _, err := Chat(ctx, config.Provider{Type: "openai", BaseURL: srv.URL + "/limited"}, "s", "u"); !errors.Is(err, ErrRateLimit) {
		t.Fatal(err)
	}
	if _, err := Chat(ctx, config.Provider{Type: "openai", BaseURL: srv.URL, APIKey: "bad"}, "s", "u"); err == nil {
		t.Fatal("want error")
	}
}
