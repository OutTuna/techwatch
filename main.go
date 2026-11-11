package main

import (
	"fmt"
	"os"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !config.Exists() || len(os.Args) > 1 && os.Args[1] == "setup" {
		res, err := tea.NewProgram(ui.NewSetup(cfg)).Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !res.(ui.Setup).Done {
			return
		}
	}
	if _, err := tea.NewProgram(ui.New(cfg), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
