package article

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const page = `<html><head><title>x</title><script>var a = "not text paragraph that is long enough";</script></head><body>
<nav><ul><li>Home page link that is quite long for a menu</li></ul></nav>
<article class="card"><p>Related story teaser that is long enough to count</p></article>
<main><article>
  <header><h1>Site header title</h1></header>
  <h2>What happened</h2>
  <p>Attackers exploited a zero-day in the VPN appliance, the vendor said on Tuesday.</p>
  <p>Share</p>
  <figure><figcaption>Photo caption that should be skipped entirely</figcaption></figure>
  <p>The flaw, tracked as CVE-2026-1234, allows remote code execution<br>without authentication.</p>
  <ul><li>Patch to version 7.2.1 or later as soon as possible.</li></ul>
</article></main>
<footer><p>Copyright footer text that is long enough to count</p></footer>
</body></html>`

func TestParse(t *testing.T) {
	ps, err := Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range ps {
		got = append(got, p.Text)
	}
	want := []string{
		"What happened",
		"Attackers exploited a zero-day in the VPN appliance, the vendor said on Tuesday.",
		"The flaw, tracked as CVE-2026-1234, allows remote code execution without authentication.",
		"Patch to version 7.2.1 or later as soon as possible.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	if !ps[0].Head || ps[1].Head {
		t.Fatal("heading flag")
	}
}

func TestParsePlainSummaryAndSplit(t *testing.T) {
	ps, _ := Parse(strings.NewReader("Short feed summary with <b>bold</b> text."))
	if len(ps) != 1 || ps[0].Text != "Short feed summary with bold text." {
		t.Fatalf("%+v", ps)
	}
	long := strings.Repeat("Attackers used the bug to steal data. ", 80)
	ps, _ = Parse(strings.NewReader("<p>" + long + "</p>"))
	if len(ps) < 3 {
		t.Fatal("long paragraph not split", len(ps))
	}
	for _, p := range ps {
		if len(p.Text) > maxLen || !strings.HasSuffix(p.Text, ".") {
			t.Fatalf("bad piece %d %q", len(p.Text), p.Text[len(p.Text)-10:])
		}
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cp1251":
			w.Header().Set("Content-Type", "text/html; charset=windows-1251")
			// "Вразливість у роутерах дозволяє зламати мережу" in windows-1251
			w.Write([]byte("<p>\xc2\xf0\xe0\xe7\xeb\xe8\xe2\xb3\xf1\xf2\xfc \xf3 \xf0\xee\xf3\xf2\xe5\xf0\xe0\xf5 \xe4\xee\xe7\xe2\xee\xeb\xff\xba \xe7\xeb\xe0\xec\xe0\xf2\xe8 \xec\xe5\xf0\xe5\xe6\xf3</p>"))
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
		default:
			w.WriteHeader(403)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	ps, err := Fetch(ctx, srv.URL+"/cp1251")
	if err != nil || len(ps) != 1 || ps[0].Text != "Вразливість у роутерах дозволяє зламати мережу" {
		t.Fatal(ps, err)
	}
	if _, err := Fetch(ctx, srv.URL+"/pdf"); err != ErrNotHTML {
		t.Fatal(err)
	}
	if _, err := Fetch(ctx, srv.URL+"/x"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatal(err)
	}
}

// TECHWATCH_LIVE=1 TECHWATCH_URL=https://... go test ./article -run Live -v
func TestLive(t *testing.T) {
	u := os.Getenv("TECHWATCH_URL")
	if os.Getenv("TECHWATCH_LIVE") == "" || u == "" {
		t.Skip("set TECHWATCH_LIVE=1 and TECHWATCH_URL")
	}
	ps, err := Fetch(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range ps[:min(6, len(ps))] {
		t.Logf("%d head=%v %.120s", i, p.Head, p.Text)
	}
	t.Logf("paragraphs: %d", len(ps))
}
