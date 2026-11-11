package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Unofficial endpoint, no key. Keep it slow or it starts returning 429.
var googleURL = "https://translate.googleapis.com/translate_a/single"

var client = &http.Client{Timeout: 20 * time.Second}

type limiter struct {
	mu    sync.Mutex
	next  time.Time
	every time.Duration
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := later(l.next, now)
	l.next = at.Add(l.every)
	l.mu.Unlock()
	select {
	case <-time.After(at.Sub(now)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

var googleLimit = &limiter{every: 700 * time.Millisecond}

func google(ctx context.Context, text, lang string) (string, error) {
	if err := googleLimit.wait(ctx); err != nil {
		return "", err
	}
	q := url.Values{"client": {"gtx"}, "sl": {"auto"}, "tl": {lang}, "dt": {"t"}, "q": {text}}
	req, err := http.NewRequestWithContext(ctx, "GET", googleURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return parseGoogle(body)
}

// Response looks like [[["перекладений","original",...],...],...].
func parseGoogle(body []byte) (string, error) {
	var raw []any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("google: empty response")
	}
	segs, ok := raw[0].([]any)
	if !ok {
		return "", fmt.Errorf("google: unexpected response")
	}
	var b strings.Builder
	for _, s := range segs {
		if parts, ok := s.([]any); ok && len(parts) > 0 {
			if t, ok := parts[0].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String(), nil
}

// googleMany joins titles with newlines to save requests, falls back to one by one if lines don't match.
// Any request error stops it: the rest stays untranslated.
func googleMany(ctx context.Context, titles []string, lang string) map[string]string {
	out := map[string]string{}
	for _, chunk := range chunks(titles, 1500) {
		if len(chunk) > 1 {
			res, err := google(ctx, strings.Join(chunk, "\n"), lang)
			lines := strings.Split(strings.TrimSpace(res), "\n")
			if err == nil && len(lines) == len(chunk) {
				for i, s := range chunk {
					out[s] = strings.TrimSpace(lines[i])
				}
				continue
			}
			if err != nil {
				return out
			}
		}
		for _, s := range chunk {
			res, err := google(ctx, s, lang)
			if err != nil {
				return out
			}
			if res != "" {
				out[s] = res
			}
		}
	}
	return out
}

func chunks(titles []string, maxLen int) [][]string {
	var out [][]string
	var cur []string
	n := 0
	for _, s := range titles {
		if len(cur) > 0 && n+len(s) > maxLen {
			out = append(out, cur)
			cur, n = nil, 0
		}
		cur = append(cur, s)
		n += len(s) + 1
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}
