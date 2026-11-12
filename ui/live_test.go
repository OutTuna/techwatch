package ui

import (
	"os"
	"testing"

	"github.com/OutTuna/techwatch/config"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// TECHWATCH_LIVE=1 go test ./ui -run Live -v  — prints every tab with real data
func TestLiveRender(t *testing.T) {
	if os.Getenv("TECHWATCH_LIVE") == "" {
		t.Skip("set TECHWATCH_LIVE=1")
	}
	t.Setenv("HOME", t.TempDir())
	var m tea.Model = New(config.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	mm := m.(Model)
	var cmd tea.Cmd
	for _, msg := range runBatch(mm.fetch()) {
		m, cmd = mm.Update(msg)
		for _, msg := range runBatch(cmd) {
			m, _ = m.Update(msg)
		}
	}
	m, _ = m.Update(m.(Model).fetchWeather()())
	for _, k := range []string{"1", "2", "3", "4", "5", "6"} {
		m = press(m, k)
		t.Log("\n" + m.View())
	}
	// article panel on the first security item
	m, cmd = press(m, "2").Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	for _, msg := range runBatch(cmd) {
		if am, ok := msg.(articleMsg); ok {
			m, cmd = m.Update(am)
			for _, msg := range runBatch(cmd) {
				if tm, ok := msg.(articleTransMsg); ok {
					m, _ = m.Update(tm)
				}
			}
		}
	}
	t.Log("\n" + m.View())
}

// runBatch runs a command and the commands inside a batch, skipping ticks.
func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	b, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range b {
		if c == nil {
			continue
		}
		if m := c(); m != nil {
			if _, tick := m.(spinner.TickMsg); !tick {
				out = append(out, m)
			}
		}
	}
	return out
}
