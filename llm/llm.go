package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OutTuna/techwatch/config"
)

var ErrRateLimit = errors.New("rate limited")

var client = &http.Client{Timeout: 90 * time.Second}

// Chat sends one system+user prompt and returns the text answer.
func Chat(ctx context.Context, p config.Provider, system, user string) (string, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	if p.Type == "gemini" {
		body := map[string]any{
			"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}},
			"contents":          []map[string]any{{"role": "user", "parts": []map[string]string{{"text": user}}}},
			"generationConfig":  map[string]any{"temperature": 0.2},
		}
		var out struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		hdr := map[string]string{"x-goog-api-key": p.Key()}
		if err := post(ctx, base+"/models/"+p.Model+":generateContent", hdr, body, &out); err != nil {
			return "", err
		}
		if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
			return "", errors.New("empty answer")
		}
		return out.Candidates[0].Content.Parts[0].Text, nil
	}

	body := map[string]any{
		"model":       p.Model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	hdr := map[string]string{}
	if k := p.Key(); k != "" {
		hdr["Authorization"] = "Bearer " + k
	}
	if err := post(ctx, base+"/chat/completions", hdr, body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("empty answer")
	}
	return out.Choices[0].Message.Content, nil
}

func post(ctx context.Context, url string, hdr map[string]string, in, out any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimit
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
