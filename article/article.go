// Package article pulls readable text out of a news page.
package article

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type Para struct {
	Text string
	Head bool
}

const (
	maxParas = 150
	maxLen   = 1200 // longer paragraphs are cut at sentences, keeps translate requests short
	minLen   = 25   // shorter text is mostly "Share", "Advertisement" and such
)

var (
	ErrNotHTML = errors.New("not an html page")
	client     = &http.Client{Timeout: 20 * time.Second}
)

func Fetch(ctx context.Context, url string) ([]Para, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	// some sites answer 403 to unknown agents
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if mt, _, _ := mime.ParseMediaType(ct); mt != "text/html" && mt != "application/xhtml+xml" {
		return nil, ErrNotHTML
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, 5<<20), ct)
	if err != nil {
		return nil, err
	}
	return Parse(body)
}

// Parse takes the <article> or <main> with the most paragraph text, <body> if there is none.
// Plain text without paragraphs (feed summaries) comes back as one paragraph.
func Parse(r io.Reader) ([]Para, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	best := pick(findAll(doc, "article", "main"))
	if len(best) == 0 {
		best = pick(findAll(doc, "body")) // body also holds menus and footers, so only as fallback
	}
	if len(best) == 0 {
		if t := strings.Join(strings.Fields(textOf(doc)), " "); t != "" {
			best = split(t)
		}
	}
	return best[:min(len(best), maxParas)], nil
}

func pick(nodes []*html.Node) []Para {
	var best []Para
	for _, n := range nodes {
		if ps := collect(n); textLen(ps) > textLen(best) {
			best = ps
		}
	}
	return best
}

func findAll(n *html.Node, tags ...string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, t := range tags {
				if n.Data == t {
					out = append(out, n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func collect(root *html.Node) []Para {
	var out []Para
	seen := map[string]bool{}
	add := func(text string, head bool) {
		text = strings.Join(strings.Fields(text), " ")
		if utf8.RuneCountInString(text) < minLen && !(head && text != "") {
			return
		}
		if seen[text] {
			return
		}
		seen[text] = true
		for _, s := range split(text) {
			out = append(out, Para{s.Text, head})
		}
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "nav", "aside", "footer", "header", "form", "figure", "button", "svg", "iframe":
				return
			case "p", "li", "blockquote", "pre":
				add(textOf(n), false)
				return
			case "h1", "h2", "h3", "h4":
				add(textOf(n), true)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "button" || n.Data == "svg") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			b.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func textLen(ps []Para) int {
	n := 0
	for _, p := range ps {
		n += len(p.Text)
	}
	return n
}

// split cuts long text at ". " near maxLen.
func split(t string) []Para {
	var out []Para
	for len(t) > maxLen {
		cut := strings.LastIndex(t[:maxLen], ". ")
		if cut < maxLen/3 {
			cut = strings.LastIndex(t[:maxLen], " ")
		}
		if cut <= 0 {
			break
		}
		out = append(out, Para{Text: strings.TrimSpace(t[:cut+1])})
		t = strings.TrimSpace(t[cut+1:])
	}
	return append(out, Para{Text: t})
}
