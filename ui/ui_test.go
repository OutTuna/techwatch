package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/digest"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/llm"
	"github.com/OutTuna/techwatch/weather"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m, _ = m.Update(msg)
	}
	return m
}

func testModel(t *testing.T) Model {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	m := New(config.Default())
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = mm.(Model)
	now := time.Now()
	mm, _ = m.Update(fetchedMsg{gen: m.gen, items: []feeds.Item{
		{Title: "Zero-day in X", Category: "security", Source: "A", Level: feeds.Critical, Published: now, URL: "https://a/1", Lang: "en"},
		{Title: "Breach at Y", Category: "security", Source: "B", Level: feeds.High, Published: now, URL: "https://a/2", Lang: "en"},
		{Title: "Ransomware Z", Category: "security", Source: "A", Level: feeds.High, Published: now, URL: "https://a/3", Lang: "en"},
		{Title: "KEV item", Category: "vulns", Source: "CISA KEV", Level: feeds.Critical, Published: now, CVEs: []string{"CVE-2026-1"}, Lang: "en"},
		{Title: "o/r v1", Category: "releases", Source: "o/r", Level: feeds.Low, Published: now, Lang: "en"},
		{Title: "Новина", Category: "local", Source: "DOU", Published: now, Lang: "uk"},
		{Title: "Gadget", Category: "tech", Source: "Verge", Published: now.Add(-time.Hour), Lang: "en"},
	}})
	return mm.(Model)
}

func TestTabsAndNavigation(t *testing.T) {
	m := testModel(t)
	if len(m.rows) != 3 { // overview: 2 critical + 1 release
		t.Fatalf("overview rows %d", len(m.rows))
	}
	m = press(m, "2").(Model)
	if m.tab != tabSecurity || len(m.rows) != 3 {
		t.Fatal(m.tab, len(m.rows))
	}
	m = press(m, "j", "j", "j", "j").(Model)
	if m.sel[tabSecurity] != 2 {
		t.Fatal("j must stop at last row", m.sel[tabSecurity])
	}
	m = press(m, "g").(Model)
	if m.sel[tabSecurity] != 0 {
		t.Fatal("g")
	}
	m = press(m, "G").(Model)
	if m.sel[tabSecurity] != 2 || m.vp.YOffset == 0 {
		t.Fatal("G must scroll to last row", m.vp.YOffset)
	}
	m = press(m, "tab", "tab", "tab").(Model)
	if m.tab != tabTech || len(m.rows) != 2 {
		t.Fatal(m.tab, len(m.rows))
	}
}

func TestFilter(t *testing.T) {
	m := press(testModel(t), "5", "f").(Model) // tech tab: all -> tech
	if m.filter[tabTech] != "tech" || len(m.rows) != 1 || m.rows[0].Title != "Gadget" {
		t.Fatal(m.filter[tabTech], m.rows)
	}
	m = press(m, "2", "f").(Model) // security: one category, so cycles sources
	if m.filter[tabSecurity] != "A" || len(m.rows) != 2 {
		t.Fatal(m.filter[tabSecurity], len(m.rows))
	}
	m = press(m, "f", "f").(Model)
	if m.filter[tabSecurity] != "" || len(m.rows) != 3 {
		t.Fatal("filter must cycle back to all")
	}
}

func TestTranslationMsgs(t *testing.T) {
	m := testModel(t)
	tr := append([]feeds.Item(nil), m.items...)
	tr[0].Translated = "Нуль-день у X"
	mm, _ := m.Update(translatedMsg{gen: m.gen - 1, lang: "uk", items: tr})
	if mm.(Model).items[0].Translated != "" {
		t.Fatal("stale gen applied")
	}
	mm, _ = m.Update(translatedMsg{gen: m.gen, lang: "uk", items: tr})
	m = mm.(Model)
	view := m.View()
	if !strings.Contains(view, "Нуль-день у X") || !strings.Contains(view, "Zero-day in X") {
		t.Fatal("translation and original must both show")
	}

	m = press(m, "L").(Model)
	if m.cfg.Language != "en" || m.items[0].Translated != "" || !config.Exists() {
		t.Fatal("L must switch, clear and save")
	}
	mm, _ = m.Update(translatedMsg{gen: m.gen, lang: "uk", items: tr})
	if mm.(Model).items[0].Translated != "" {
		t.Fatal("old language result applied")
	}
}

func TestProviderSwitch(t *testing.T) {
	m := press(testModel(t), "P").(Model)
	if m.cfg.ActiveProvider != "ollama" { // only provider without key requirement
		t.Fatal(m.cfg.ActiveProvider)
	}
	if !strings.Contains(m.View(), "ollama") {
		t.Fatal("header must show provider")
	}
	m = press(m, "P").(Model)
	if m.cfg.ActiveProvider != "" || !strings.Contains(m.View(), "FREE") {
		t.Fatal("back to FREE")
	}
}

func TestOpenURLRejects(t *testing.T) {
	for _, u := range []string{"", "file:///etc/passwd", "/Applications/Calculator.app", "javascript:x"} {
		if openURL(u) == nil {
			t.Fatal("accepted", u)
		}
	}
}

func TestSetup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := config.Default()
	cfg.Releases = nil
	var m tea.Model = NewSetup(cfg)
	// lang: en (k from uk) ; provider: groq (2nd) ; key ; releases: toggle first two ; custom
	m = press(m, "k", "enter", "j", "enter", "s", "e", "c", "enter", " ", "j", " ", "enter")
	s := m.(Setup)
	if s.step != stepCustom {
		t.Fatal("step", s.step)
	}
	m = press(m, "b", "a", "d", "enter")
	if m.(Setup).err == "" || m.(Setup).Done {
		t.Fatal("invalid repo must be rejected")
	}
	for range 3 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m = press(m, "a", "/", "b", "enter")
	if m.(Setup).step != stepCity || m.(Setup).Done {
		t.Fatal("city step expected")
	}
	m = press(m, "enter") // city unchanged: no lookup, save
	s = m.(Setup)
	if !s.Done {
		t.Fatal("not done:", s.err)
	}
	c, _ := config.Load()
	if c.Language != "en" || c.ActiveProvider != "groq" || c.Active().APIKey != "sec" ||
		strings.Join(c.Releases, ",") != "torvalds/linux,golang/go,a/b" {
		t.Fatalf("%+v", c)
	}
}

func TestSetupSkipsKeyForOllama(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var m tea.Model = NewSetup(config.Default())
	m = press(m, "enter", "j", "j", "j", "enter")
	if m.(Setup).step != stepReleases {
		t.Fatal(m.(Setup).step)
	}
	m = press(m, "esc")
	if m.(Setup).step != stepProvider {
		t.Fatal("esc must skip key step back")
	}
}

func TestSetupGeocodeResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewSetup(config.Default())
	s.step = stepCity
	m, _ := s.Update(geoMsg{err: errors.New("city not found: X")})
	if m.(Setup).err == "" || m.(Setup).Done {
		t.Fatal("error must keep setup open")
	}
	m, _ = m.Update(geoMsg{place: &weather.Place{Name: "Львів", Country: "UA", Lat: 49.8, Lon: 24}})
	c, _ := config.Load()
	if !m.(Setup).Done || c.Location.City != "Львів" || c.Location.Latitude != 49.8 {
		t.Fatal(c.Location)
	}
}

func TestWeatherTab(t *testing.T) {
	m := testModel(t)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm, _ = mm.Update(weatherMsg{now: &weather.Now{Temp: 3.4, Code: 61, IsDay: true, WindDir: 180},
		days: []weather.Day{{Date: time.Now(), Min: 1, Max: 5, Code: 61, Rain: 2, RainProb: 80}, {Date: time.Now().Add(24 * time.Hour), Min: -0.3, Max: 3}}})
	v := press(mm, "6").View()
	for _, want := range []string{"3°C", "Дощ", "Сьогодні", "0°C", "80%", "━"} {
		if !strings.Contains(v, want) {
			t.Fatalf("weather tab missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "-0°C") {
		t.Fatal("negative zero")
	}
	mm, _ = mm.Update(weatherMsg{err: errors.New("HTTP 500")})
	if !strings.Contains(press(mm, "6").View(), "HTTP 500") {
		t.Fatal("weather error not shown")
	}
}

func TestDigestPanel(t *testing.T) {
	m := testModel(t)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	if !strings.Contains(mm.View(), "console.groq.com") {
		t.Fatal("FREE hint missing")
	}
	mm = press(mm, "P") // ollama
	m = mm.(Model)
	if !m.digBusy {
		t.Fatal("P must start digest")
	}
	mm, _ = m.Update(digestMsg{d: &digest.Digest{Text: "Головне за день", Lang: "uk", Provider: "ollama", At: time.Now()}, lang: "uk", provider: "ollama"})
	if v := mm.View(); !strings.Contains(v, "Головне за день") || !strings.Contains(v, "ollama") {
		t.Fatal("digest not shown")
	}
	if digest.Load("uk", "ollama", time.Hour) == nil {
		t.Fatal("digest not cached")
	}
	mm, _ = mm.Update(digestMsg{err: llm.ErrRateLimit, lang: "uk", provider: "ollama"})
	if !strings.Contains(mm.View(), "429") {
		t.Fatal("rate limit hint missing")
	}
}

func TestFirstFetchAccepted(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New(config.Default())
	// Init has a value receiver: the gen it fetches with must already be in the model
	mm, _ := m.Update(fetchedMsg{gen: m.gen, items: []feeds.Item{{Title: "x", Category: "security"}}})
	if mm.(Model).fetching || len(mm.(Model).items) != 1 {
		t.Fatal("first fetch result dropped")
	}
}

func TestCleanRemoteText(t *testing.T) {
	m := testModel(t)
	m.items[0].Translated = "Злам\x1b]52;c;ZXZpbA==\x07\nдругий рядок"
	m.items[0].Source = "A\x1b[2J"
	m.render()
	v := m.View()
	if strings.Contains(v, "\x1b]52") || strings.Contains(v, "\x07") || strings.Contains(v, "\x1b[2J") {
		t.Fatal("control sequence reached the terminal")
	}
	if len(m.itemLines(m.items[0], false)) != 3 {
		t.Fatal("newline in title must not add a row line")
	}
	if clean("a\nb", true) != "a\nb" || clean("a\tb\u009b", false) != "a b" {
		t.Fatal("clean")
	}
}

func TestIdleSpinnerStops(t *testing.T) {
	m := testModel(t)
	m.trans = false
	if _, cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Fatal("idle spinner must stop ticking")
	}
}

func TestOverviewIgnoresReleasesFilter(t *testing.T) {
	m := testModel(t)
	m.items = append(m.items, feeds.Item{Title: "x/y v2", Category: "releases", Source: "x/y", Published: time.Now()})
	m.filter[tabReleases] = "o/r"
	m.render()
	var rel int
	for _, it := range m.rows {
		if it.Category == "releases" {
			rel++
		}
	}
	if rel != 2 {
		t.Fatal("overview releases filtered:", rel)
	}
}
