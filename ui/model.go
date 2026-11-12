package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/digest"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/i18n"
	"github.com/OutTuna/techwatch/llm"
	"github.com/OutTuna/techwatch/translate"
	"github.com/OutTuna/techwatch/weather"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	tabOverview = iota
	tabSecurity
	tabCVE
	tabReleases
	tabTech
	tabWeather
	tabCount
)

var tabKeys = [tabCount]string{"tab.overview", "tab.security", "tab.cve", "tab.releases", "tab.tech", "tab.weather"}

// categories shown on each list tab
var tabCats = map[int][]string{
	tabSecurity: {"security"},
	tabCVE:      {"vulns"},
	tabReleases: {"releases"},
	tabTech:     {"tech", "local"},
}

type (
	fetchedMsg struct {
		gen    int
		items  []feeds.Item
		failed []string
	}
	translatedMsg struct {
		gen   int
		lang  string
		items []feeds.Item
	}
	digestMsg struct {
		d              *digest.Digest
		err            error
		lang, provider string
	}
	weatherMsg struct {
		now  *weather.Now
		days []weather.Day
		err  error
	}
	tickMsg time.Time
)

type Model struct {
	cfg *config.Config
	tr  *translate.Translator

	w, h     int
	tab      int
	items    []feeds.Item
	gen      int
	fetching bool
	trans    bool
	updated  time.Time
	status   string

	dig     *digest.Digest
	digErr  error
	digBusy bool
	wNow    *weather.Now
	wDays   []weather.Day
	wErr    error

	sel    [tabCount]int
	filter [tabCount]string
	rows   []feeds.Item // selectable rows of the current tab
	rowAt  []int        // first line of each row
	vp     viewport.Model
	spin   spinner.Model
}

func New(cfg *config.Config) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	// first fetch starts in Init, which can't change the model
	return Model{cfg: cfg, tr: translate.New(cfg), vp: viewport.New(80, 20), spin: sp, gen: 1, fetching: true}
}

func (m Model) t(key string) string { return i18n.T(m.cfg.Language, key) }

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchCmd(m.gen, feeds.Sources(m.cfg)), m.fetchWeather(), m.spin.Tick, tick(m.cfg.RefreshSec))
}

func tick(sec int) tea.Cmd {
	return tea.Tick(time.Duration(sec)*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) fetch() tea.Cmd {
	m.gen++
	m.fetching = true
	return tea.Batch(fetchCmd(m.gen, feeds.Sources(m.cfg)), m.spin.Tick)
}

func fetchCmd(gen int, srcs []config.Feed) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		items, failed := feeds.Fetch(ctx, srcs)
		return fetchedMsg{gen, items, failed}
	}
}

func (m Model) fetchWeather() tea.Cmd {
	loc, unit := m.cfg.Location, m.cfg.TempUnit
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		now, days, err := weather.Fetch(ctx, loc.Latitude, loc.Longitude, unit)
		return weatherMsg{now, days, err}
	}
}

// digestCmd uses the cached digest or asks the active LLM for a new one.
func (m *Model) digestCmd() tea.Cmd {
	p := m.cfg.Active()
	if p == nil || len(m.items) == 0 || m.digBusy {
		return nil
	}
	maxAge := time.Duration(m.cfg.DigestCacheMins) * time.Minute
	if d := digest.Load(m.cfg.Language, p.Name, maxAge); d != nil {
		m.dig, m.digErr = d, nil
		return nil
	}
	if m.dig != nil && m.dig.Lang == m.cfg.Language && m.dig.Provider == p.Name && time.Since(m.dig.At) < maxAge {
		return nil
	}
	m.digBusy, m.digErr = true, nil
	pc, lang, items := *p, m.cfg.Language, slices.Clone(m.items)
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		d, err := digest.Generate(ctx, pc, lang, items)
		return digestMsg{d, err, lang, pc.Name}
	})
}

func (m *Model) translate() tea.Cmd {
	m.trans = true
	tr := *m.tr // copy: provider may change while this runs
	if p := m.cfg.Active(); p != nil {
		pc := *p
		tr.Provider = &pc
	}
	gen, lang, items := m.gen, m.cfg.Language, slices.Clone(m.items)
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return translatedMsg{gen, lang, tr.Translate(ctx, items, lang)}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.vp.Width, m.vp.Height = msg.Width, max(1, msg.Height-4)

	case fetchedMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.fetching, m.updated = false, time.Now()
		m.items = keepTranslations(msg.items, m.items)
		m.status = m.t("status.refreshed")
		if len(msg.failed) > 0 {
			m.status += " · " + m.t("status.unavailable") + ": " + strings.Join(msg.failed, ", ")
		}
		cmd = tea.Batch(m.translate(), m.digestCmd())

	case digestMsg:
		m.digBusy = false
		if p := m.cfg.Active(); p != nil && p.Name == msg.provider && msg.lang == m.cfg.Language {
			m.dig, m.digErr = msg.d, msg.err
			if msg.err == nil {
				_ = digest.Save(msg.d)
			}
		} else {
			cmd = m.digestCmd() // settings changed while it ran
		}

	case weatherMsg:
		m.wNow, m.wDays, m.wErr = msg.now, msg.days, msg.err

	case translatedMsg:
		if msg.gen == m.gen && msg.lang == m.cfg.Language {
			m.items, m.trans = msg.items, false
		}

	case tickMsg:
		cmd = tea.Batch(m.fetch(), m.fetchWeather(), tick(m.cfg.RefreshSec))

	case spinner.TickMsg:
		if !m.fetching && !m.trans && !m.digBusy {
			return m, nil // idle: stop ticking, a new job restarts it
		}
		m.spin, cmd = m.spin.Update(msg)

	case tea.KeyMsg:
		cmd = m.key(msg.String())
	}
	m.render()
	return m, cmd
}

func (m *Model) key(k string) tea.Cmd {
	n := len(m.rows)
	s := &m.sel[m.tab]
	switch k {
	case "q", "ctrl+c":
		return tea.Quit
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % tabCount
	case "shift+tab", "left", "h":
		m.tab = (m.tab + tabCount - 1) % tabCount
	case "1", "2", "3", "4", "5", "6":
		m.tab = int(k[0] - '1')
	case "j", "down":
		*s = min(*s+1, n-1)
	case "k", "up":
		*s = max(*s-1, 0)
	case "d", "pgdown":
		*s = min(*s+m.pageRows(), n-1)
	case "u", "pgup":
		*s = max(*s-m.pageRows(), 0)
	case "g", "home":
		*s = 0
	case "G", "end":
		*s = n - 1
	case "enter":
		if *s >= 0 && *s < n {
			if err := openURL(m.rows[*s].URL); err == nil {
				m.status = m.t("status.opened")
			} else if errors.Is(err, errBadURL) {
				m.status = m.t("status.bad_url")
			} else {
				m.status = m.t("status.error") + ": " + err.Error()
			}
		}
	case "r":
		if !m.fetching {
			return tea.Batch(m.fetch(), m.fetchWeather())
		}
	case "f":
		opts := m.filterOptions()
		m.filter[m.tab] = opts[(slices.Index(opts, m.filter[m.tab])+1)%len(opts)]
		*s = 0
		m.status = m.t("status.filter") + ": " + m.filterLabel()
	case "L":
		m.cfg.Language = i18n.Next(m.cfg.Language)
		for i := range m.items {
			m.items[i].Translated = ""
		}
		m.status = m.t("status.lang") + ": " + m.t("lang.name") + m.save()
		return tea.Batch(m.translate(), m.digestCmd())
	case "P":
		m.cfg.NextProvider()
		m.status = m.t("status.provider") + ": " + m.modeName() + m.save()
		return m.digestCmd()
	}
	*s = max(0, min(*s, n-1))
	return nil
}

func (m *Model) save() string {
	if err := config.Save(m.cfg); err != nil {
		return " · " + m.t("status.error") + ": " + err.Error()
	}
	return ""
}

// keepTranslations reuses old translations so a refresh doesn't flash originals.
func keepTranslations(fresh, old []feeds.Item) []feeds.Item {
	tr := map[string]string{}
	for _, it := range old {
		if it.Translated != "" {
			tr[it.Title] = it.Translated
		}
	}
	for i := range fresh {
		fresh[i].Translated = tr[fresh[i].Title]
	}
	return fresh
}

func (m Model) pageRows() int { return max(1, m.vp.Height/6) }

func (m Model) modeName() string {
	if p := m.cfg.Active(); p != nil {
		return p.Name
	}
	return m.t("status.free")
}

// tabItems: items of the tab after filter. Filter cycles categories, or sources when the tab has one category.
func (m Model) tabItems(tab int) []feeds.Item {
	var out []feeds.Item
	f := m.filter[tab]
	for _, it := range m.items {
		if slices.Contains(tabCats[tab], it.Category) && (f == "" || f == it.Category || f == it.Source) {
			out = append(out, it)
		}
	}
	if tab == tabReleases || tab == tabTech {
		sort.SliceStable(out, func(i, j int) bool { return out[i].Published.After(out[j].Published) })
	}
	return out
}

func (m Model) filterOptions() []string {
	opts := []string{""}
	if cats := tabCats[m.tab]; len(cats) > 1 {
		return append(opts, cats...)
	}
	for _, it := range m.items {
		if slices.Contains(tabCats[m.tab], it.Category) && !slices.Contains(opts, it.Source) {
			opts = append(opts, it.Source)
		}
	}
	sort.Strings(opts[1:])
	return opts
}

func (m Model) filterLabel() string {
	f := m.filter[m.tab]
	if f == "" {
		return m.t("cat.all")
	}
	if slices.Contains(feeds.Categories, f) {
		return m.t("cat." + f)
	}
	return f
}

// render rebuilds the current tab content and keeps the selected row visible.
func (m *Model) render() {
	var b []string
	m.rows, m.rowAt = nil, nil
	add := func(items []feeds.Item) {
		for _, it := range items {
			m.rowAt = append(m.rowAt, len(b))
			b = append(b, m.itemLines(it, len(m.rows) == m.sel[m.tab])...)
			m.rows = append(m.rows, it)
		}
	}
	empty := func() { b = append(b, styleDim.Render("  "+m.t("status.empty"))) }

	switch m.tab {
	case tabOverview:
		var crit, rel []feeds.Item
		for _, it := range m.items {
			if it.Level == feeds.Critical && len(crit) < 10 {
				crit = append(crit, it)
			}
		}
		for _, it := range m.items {
			if it.Category == "releases" {
				rel = append(rel, it)
			}
		}
		sort.SliceStable(rel, func(i, j int) bool { return rel[i].Published.After(rel[j].Published) })
		rel = rel[:min(8, len(rel))]
		b = append(b, "", styleHead.Render(m.t("overview.critical")))
		if add(crit); len(crit) == 0 {
			empty()
		}
		b = append(b, "", styleHead.Render(m.t("overview.releases")))
		if add(rel); len(rel) == 0 {
			empty()
		}
		b = append(b, "", styleHead.Render(m.t("overview.digest")))
		b = append(b, m.digestLines()...)
	case tabWeather:
		b = append(b, m.weatherLines()...)
	default:
		items := m.tabItems(m.tab)
		if m.filter[m.tab] != "" {
			b = append(b, styleDim.Render(m.t("status.filter")+": "+m.filterLabel()))
		}
		if add(items); len(items) == 0 {
			empty()
		}
	}

	if n := len(m.rows); n > 0 && m.sel[m.tab] >= n {
		m.sel[m.tab] = n - 1
		m.render()
		return
	}
	m.vp.SetContent(strings.Join(b, "\n"))
	if s := m.sel[m.tab]; s < len(m.rowAt) {
		top, bottom := m.rowAt[s], len(b)-1
		if s+1 < len(m.rowAt) {
			bottom = m.rowAt[s+1] - 1
		}
		if s == 0 {
			top = 0 // show section header too
		}
		if top < m.vp.YOffset {
			m.vp.SetYOffset(top)
		} else if bottom >= m.vp.YOffset+m.vp.Height {
			m.vp.SetYOffset(bottom - m.vp.Height + 1)
		}
	} else {
		m.vp.GotoTop()
	}
}

// clean drops control chars from remote text: ESC could run terminal commands, \n would break row layout.
func clean(s string, keepNL bool) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && keepNL:
			return r
		case r == '\n' || r == '\t' || r == '\r':
			return ' '
		case r < 0x20 || r >= 0x7f && r <= 0x9f:
			return -1
		}
		return r
	}, s)
}

func (m Model) itemLines(it feeds.Item, selected bool) []string {
	w := max(20, m.w)
	cursor, title := "  ", clean(it.Title, false)
	if it.Translated != "" {
		title = clean(it.Translated, false)
	}
	if selected {
		cursor, title = "▶ ", styleSel.Render(title)
	}
	meta := clean(it.Source, false) + " · " + age(m.cfg.Language, it.Published)
	for _, c := range it.CVEs {
		meta += " " + styleCVE.Render("["+c+"]")
	}
	lines := []string{
		ansi.Truncate(cursor+badge(it.Level)+" "+title, w, "…"),
		ansi.Truncate("       "+styleDim.Render(meta), w, "…"),
	}
	if m.cfg.ShowOriginal && it.Translated != "" && it.Translated != it.Title {
		lines = append(lines, ansi.Truncate("       "+styleDim.Render(clean(it.Title, false)), w, "…"))
	}
	return lines
}

func (m Model) digestLines() []string {
	switch {
	case m.cfg.Active() == nil:
		return strings.Split(styleDim.Render(m.t("digest.free_hint")), "\n")
	case m.digBusy:
		return []string{"  " + m.spin.View() + m.t("status.loading")}
	case m.digErr != nil:
		msg := m.t("digest.error") + ": " + clean(m.digErr.Error(), false)
		if errors.Is(m.digErr, llm.ErrRateLimit) {
			msg = m.t("digest.rate_limit")
		} else if errors.Is(m.digErr, digest.ErrNoNews) {
			msg = m.t("digest.no_news")
		}
		return []string{ansi.Truncate("  "+styleErr.Render(msg), m.w, "…")}
	case m.dig == nil:
		return []string{styleDim.Render("  " + m.t("digest.pending"))}
	}
	text := lipgloss.NewStyle().Width(max(20, m.w-4)).Render(clean(m.dig.Text, true))
	var out []string
	for _, l := range strings.Split(text, "\n") {
		out = append(out, "  "+l)
	}
	return append(out, styleDim.Render("  "+m.t("digest.generated")+" "+m.dig.At.Format("15:04")+" · "+m.dig.Provider))
}

func (m Model) temp(v float64) string {
	if v = math.Round(v); v == 0 {
		v = 0 // no "-0"
	}
	if m.cfg.TempUnit == "fahrenheit" {
		return fmt.Sprintf("%.0f°F", v)
	}
	return fmt.Sprintf("%.0f°C", v)
}

func (m Model) weatherLines() []string {
	if m.wErr != nil {
		return []string{"", "  " + styleErr.Render(m.t("status.error")+": "+clean(m.wErr.Error(), false))}
	}
	if m.wNow == nil {
		return []string{"", "  " + m.spin.View() + m.t("status.loading")}
	}
	n := m.wNow
	icon, desc := weather.Describe(n.Code, n.IsDay)
	loc := m.cfg.Location
	out := []string{
		"",
		"  " + styleDim.Render(loc.City+", "+loc.Country),
		"  " + icon + "  " + styleSel.Render(m.temp(n.Temp)) + "  " + m.t(desc),
		"  " + styleDim.Render(fmt.Sprintf("%s %s · %s %d%% · %s %.0f %s %s · UV %.0f",
			m.t("weather.feels"), m.temp(n.Feels), m.t("weather.humidity"), n.Humidity,
			m.t("weather.wind"), n.Wind, m.t("weather.kmh"), weather.Compass(n.WindDir), n.UV)),
		"",
		styleHead.Render(m.t("weather.forecast")),
	}
	if len(m.wDays) == 0 {
		return out
	}
	lo, hi := m.wDays[0].Min, m.wDays[0].Max
	for _, d := range m.wDays {
		lo, hi = min(lo, d.Min), max(hi, d.Max)
	}
	const barW = 20
	pos := func(v float64) int {
		if hi == lo {
			return 0
		}
		return int((v - lo) / (hi - lo) * (barW - 1))
	}
	for i, d := range m.wDays {
		label := m.t("day."+fmt.Sprint(int(d.Date.Weekday()))) + d.Date.Format(" 02")
		if i == 0 {
			label = m.t("weather.today")
		}
		a, z := pos(d.Min), pos(d.Max)
		bar := styleDim.Render(strings.Repeat("─", a)) + styleCVE.Render(strings.Repeat("━", z-a+1)) + styleDim.Render(strings.Repeat("─", barW-1-z))
		icon, _ := weather.Describe(d.Code, true)
		rain := ""
		if d.Rain > 0 || d.RainProb > 0 {
			rain = styleDim.Render(fmt.Sprintf("  %.1f mm %d%%", d.Rain, d.RainProb))
		}
		out = append(out, fmt.Sprintf("  %s %s  %5s %s %-5s%s", ansi.Truncate(label+"          ", 10, ""), icon, m.temp(d.Min), bar, m.temp(d.Max), rain))
	}
	return out
}

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	head := styleApp.Render("techwatch") + styleDim.Render(" · "+m.modeName()+" · "+m.cfg.Language)
	if !m.updated.IsZero() {
		head += styleDim.Render(" · " + m.updated.Format("15:04"))
	}
	if m.fetching {
		head += " " + m.spin.View() + m.t("status.loading")
	} else if m.trans {
		head += " " + m.spin.View() + m.t("status.translating")
	}

	var tabs []string
	for i, k := range tabKeys {
		st := styleTab
		if i == m.tab {
			st = styleTabA
		}
		tabs = append(tabs, st.Render(fmt.Sprintf("%d %s", i+1, m.t(k))))
	}

	status := clean(m.status, false)
	if strings.Contains(status, m.t("status.error")) || strings.Contains(status, m.t("status.unavailable")) {
		status = styleErr.Render(status)
	}
	return strings.Join([]string{
		ansi.Truncate(head, m.w, "…"),
		ansi.Truncate(strings.Join(tabs, ""), m.w, "…"),
		m.vp.View(),
		ansi.Truncate(status, m.w, "…"),
		ansi.Truncate(styleDim.Render(m.t("help.keys")), m.w, "…"),
	}, "\n")
}

var errBadURL = errors.New("bad url")

// openURL accepts only http(s): links come from remote feeds.
func openURL(u string) error {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return errBadURL
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", u)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() // reap the process
	return nil
}
