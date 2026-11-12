package ui

import (
	"context"
	"strings"
	"time"

	"github.com/OutTuna/techwatch/article"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// articleView is the side panel opened with "o".
type articleView struct {
	item        feeds.Item
	paras       []article.Para
	trans       []string
	err         error
	fromFeed    bool
	loading     bool
	translating bool
	vp          viewport.Model
}

type (
	articleMsg struct {
		url   string
		paras []article.Para
		err   error
	}
	articleTransMsg struct {
		url, lang string
		texts     []string
	}
)

func (m *Model) openArticle() tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	it := m.rows[m.sel[m.tab]]
	if !strings.HasPrefix(it.URL, "https://") && !strings.HasPrefix(it.URL, "http://") {
		m.status = m.t("status.bad_url")
		return nil
	}
	m.art = &articleView{item: it, loading: true, vp: viewport.New(0, 0)}
	m.layout()
	url := it.URL
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		ps, err := article.Fetch(ctx, url)
		return articleMsg{url, ps, err}
	})
}

func (m *Model) gotArticle(msg articleMsg) tea.Cmd {
	a := m.art
	if a == nil || a.item.URL != msg.url {
		return nil
	}
	a.loading, a.paras, a.err = false, msg.paras, msg.err
	if len(a.paras) == 0 && a.item.Summary != "" {
		if ps, _ := article.Parse(strings.NewReader(a.item.Summary)); len(ps) > 0 {
			a.paras, a.fromFeed = ps, true
		}
	}
	return m.translateArticle()
}

func (m *Model) translateArticle() tea.Cmd {
	a := m.art
	if a == nil || len(a.paras) == 0 {
		return nil
	}
	a.trans = nil
	if a.item.Lang == m.cfg.Language || m.cfg.TranslationProvider == "none" {
		return nil
	}
	a.translating = true
	texts := make([]string, len(a.paras))
	for i, p := range a.paras {
		texts[i] = p.Text
	}
	tr, url, lang := m.translator(), a.item.URL, m.cfg.Language
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return articleTransMsg{url, lang, tr.Texts(ctx, texts, lang)}
	})
}

func (m *Model) gotArticleTrans(msg articleTransMsg) {
	if a := m.art; a != nil && a.item.URL == msg.url && msg.lang == m.cfg.Language {
		a.translating, a.trans = false, msg.texts
	}
}

// articleKey handles keys while the panel is open; false lets the main handler take it.
func (m *Model) articleKey(k string) (tea.Cmd, bool) {
	a := m.art
	switch k {
	case "esc", "o":
		m.art = nil
		m.layout()
	case "j", "down":
		a.vp.ScrollDown(1)
	case "k", "up":
		a.vp.ScrollUp(1)
	case "d", "pgdown", " ":
		a.vp.HalfPageDown()
	case "u", "pgup":
		a.vp.HalfPageUp()
	case "g", "home":
		a.vp.GotoTop()
	case "G", "end":
		a.vp.GotoBottom()
	case "enter":
		if err := openURL(a.item.URL); err == nil {
			m.status = m.t("status.opened")
		}
	case "q", "ctrl+c", "L", "P", "r":
		return nil, false
	}
	return nil, true
}

// layout splits the body: list on the left, article on the right; narrow terminals show only the article.
func (m *Model) layout() {
	h := max(1, m.h-4)
	m.vp.Width, m.vp.Height = m.w, h
	if m.art != nil {
		pw := m.w
		if m.w >= 100 {
			pw = m.w * 55 / 100
			m.vp.Width = m.w - pw - 1
		}
		m.art.vp.Width, m.art.vp.Height = pw, h
	}
}

func (m *Model) renderArticle() {
	a := m.art
	w := max(20, a.vp.Width-2)
	wrap := lipgloss.NewStyle().Width(w)
	title := a.item.Title
	if a.item.Translated != "" {
		title = a.item.Translated
	}
	b := []string{
		wrap.Inherit(styleSel).Render(clean(title, false)),
		styleDim.Render(clean(a.item.Source, false) + " · " + age(m.cfg.Language, a.item.Published)),
		"",
	}
	note := ""
	switch {
	case a.loading:
		note = m.spin.View() + m.t("article.loading")
	case a.translating:
		note = m.spin.View() + m.t("status.translating")
	case a.fromFeed:
		note = m.t("article.from_feed")
	case a.err != nil:
		note = styleErr.Render(m.t("status.error") + ": " + clean(a.err.Error(), false))
	case len(a.paras) == 0:
		note = m.t("article.empty")
	}
	if note != "" {
		b = append(b, wrap.Inherit(styleDim).Render(note), "")
	}
	for i, p := range a.paras {
		text := p.Text
		if i < len(a.trans) && a.trans[i] != "" {
			text = a.trans[i]
		}
		st := wrap
		if p.Head {
			st = wrap.Inherit(styleHead)
		}
		b = append(b, st.Render(clean(text, false)), "")
	}
	a.vp.SetContent(strings.Join(b, "\n"))
}

func (m Model) body() string {
	if m.art == nil {
		return m.vp.View()
	}
	if m.art.vp.Width == m.w {
		return m.art.vp.View()
	}
	sep := styleDim.Render(strings.TrimSuffix(strings.Repeat("│\n", m.vp.Height), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, m.vp.View(), sep, m.art.vp.View())
}

func (m Model) busy() bool {
	a := m.art != nil && (m.art.loading || m.art.translating)
	return m.fetching || m.trans || m.digBusy || a
}
