package ui

import (
	"os"
	"testing"

	"github.com/OutTuna/techwatch/config"
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
	msg := mm.fetch()()
	m, cmd := mm.Update(msg)
	m, _ = m.Update(cmd())
	m, _ = m.Update(m.(Model).fetchWeather()())
	for _, k := range []string{"1", "2", "3", "4", "5", "6"} {
		m = press(m, k)
		t.Log("\n" + m.View())
	}
}
