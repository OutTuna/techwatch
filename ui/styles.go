package ui

import (
	"fmt"
	"time"

	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/i18n"
	"github.com/charmbracelet/lipgloss"
)

var (
	accent    = lipgloss.Color("#7aa2f7")
	styleApp  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	styleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
	styleSel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff"))
	styleHead = lipgloss.NewStyle().Bold(true).Foreground(accent)
	styleTab  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#a6adc8"))
	styleTabA = styleTab.Bold(true).Foreground(lipgloss.Color("#1e1e2e")).Background(accent)
	styleCVE  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f5c2e7"))
	styleErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8"))

	levelColors = map[feeds.Level]string{
		feeds.Critical: "#f38ba8", feeds.High: "#fab387", feeds.Medium: "#f9e2af", feeds.Low: "#89b4fa", feeds.Info: "#6c7086",
	}
	levelShort = map[feeds.Level]string{
		feeds.Critical: "CRIT", feeds.High: "HIGH", feeds.Medium: "MED ", feeds.Low: "LOW ", feeds.Info: "INFO",
	}
)

func badge(l feeds.Level) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1e1e2e")).
		Background(lipgloss.Color(levelColors[l])).Render(levelShort[l])
}

func age(lang string, t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf(i18n.T(lang, "age.min"), max(1, int(d.Minutes())))
	case d < 48*time.Hour:
		return fmt.Sprintf(i18n.T(lang, "age.hour"), int(d.Hours()))
	default:
		return fmt.Sprintf(i18n.T(lang, "age.day"), int(d.Hours()/24))
	}
}
