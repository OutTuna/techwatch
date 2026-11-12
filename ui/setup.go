package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/OutTuna/techwatch/config"
	"github.com/OutTuna/techwatch/feeds"
	"github.com/OutTuna/techwatch/i18n"
	"github.com/OutTuna/techwatch/weather"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var Presets = []string{
	"torvalds/linux", "golang/go", "nodejs/node", "python/cpython", "rust-lang/rust",
	"kubernetes/kubernetes", "moby/moby", "openssl/openssl", "nginx/nginx", "curl/curl",
	"git/git", "microsoft/vscode", "neovim/neovim", "ollama/ollama", "home-assistant/core",
}

const (
	stepLang = iota
	stepProvider
	stepKey
	stepReleases
	stepCustom
	stepCity
	stepCount
)

type Setup struct {
	cfg    *config.Config
	step   int
	cursor int
	picked map[string]bool
	input  textinput.Model
	err    string
	busy   bool
	Done   bool
}

type geoMsg struct {
	place *weather.Place
	err   error
}

func NewSetup(cfg *config.Config) Setup {
	in := textinput.New()
	in.CharLimit = 300
	s := Setup{cfg: cfg, picked: map[string]bool{}, input: in}
	for _, r := range cfg.Releases {
		s.picked[r] = true
	}
	s.cursor = slices.Index(i18n.Langs(), cfg.Language)
	return s
}

func (s Setup) t(k string) string { return i18n.T(s.cfg.Language, k) }

func (s Setup) providerOpts() []string {
	opts := []string{""}
	for _, p := range s.cfg.Providers {
		opts = append(opts, p.Name)
	}
	return opts
}

func (s Setup) listLen() int {
	switch s.step {
	case stepLang:
		return len(i18n.Langs())
	case stepProvider:
		return len(s.providerOpts())
	case stepReleases:
		return len(Presets)
	}
	return 0
}

func (s Setup) needsKey() bool {
	p := s.cfg.Active()
	return p != nil && (p.APIKeyEnv != "" || p.APIKey != "")
}

func (s Setup) Init() tea.Cmd { return textinput.Blink }

func (s Setup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if g, ok := msg.(geoMsg); ok {
		s.busy = false
		if errors.Is(g.err, weather.ErrNotFound) {
			s.err = s.t("setup.city_not_found")
			return s, nil
		}
		if g.err != nil {
			s.err = g.err.Error()
			return s, nil
		}
		s.cfg.Location = config.Location{City: g.place.Name, Country: g.place.Country, Latitude: g.place.Lat, Longitude: g.place.Lon}
		return s.finish()
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok || s.busy && k.String() != "ctrl+c" {
		return s, nil
	}
	text := s.step == stepKey || s.step == stepCustom || s.step == stepCity
	switch k.String() {
	case "ctrl+c":
		return s, tea.Quit
	case "esc":
		s.move(-1)
		return s, nil
	case "up", "k":
		if !text {
			s.cursor = max(0, s.cursor-1)
			return s, nil
		}
	case "down", "j":
		if !text {
			s.cursor = min(s.listLen()-1, s.cursor+1)
			return s, nil
		}
	case " ":
		if s.step == stepReleases {
			r := Presets[s.cursor]
			s.picked[r] = !s.picked[r]
			return s, nil
		}
	case "enter":
		return s.confirm()
	}
	if text {
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s Setup) confirm() (tea.Model, tea.Cmd) {
	s.err = ""
	switch s.step {
	case stepLang:
		s.cfg.Language = i18n.Langs()[s.cursor]
	case stepProvider:
		s.cfg.ActiveProvider = s.providerOpts()[s.cursor]
	case stepKey:
		if v := strings.TrimSpace(s.input.Value()); v != "" {
			s.cfg.Active().APIKey = v
		}
	case stepCustom:
		var custom []string
		for _, r := range strings.Split(s.input.Value(), ",") {
			if r = strings.TrimSpace(r); r == "" {
				continue
			}
			if !feeds.ValidRepo(r) {
				s.err = s.t("setup.invalid_repo") + ": " + r
				return s, nil
			}
			custom = append(custom, r)
		}
		s.cfg.Releases = nil
		for _, r := range Presets {
			if s.picked[r] {
				s.cfg.Releases = append(s.cfg.Releases, r)
			}
		}
		for _, r := range custom {
			if !slices.Contains(s.cfg.Releases, r) {
				s.cfg.Releases = append(s.cfg.Releases, r)
			}
		}
	case stepCity:
		city := strings.TrimSpace(s.input.Value())
		if city == "" || city == s.cfg.Location.City {
			return s.finish()
		}
		s.busy = true
		lang := s.cfg.Language
		return s, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			p, err := weather.Geocode(ctx, city, lang)
			return geoMsg{p, err}
		}
	}
	s.move(1)
	return s, nil
}

func (s Setup) finish() (tea.Model, tea.Cmd) {
	if p := s.cfg.Active(); p != nil && !p.Usable() {
		s.cfg.ActiveProvider = "" // picked without a key: start in FREE
	}
	if err := config.Save(s.cfg); err != nil {
		s.err = err.Error()
		return s, nil
	}
	s.Done = true
	return s, tea.Quit
}

// move goes between steps, skipping the key step when the provider needs no key.
func (s *Setup) move(d int) {
	s.step = max(0, s.step+d)
	if s.step == stepKey && !s.needsKey() {
		s.step = max(0, s.step+d)
	}
	s.cursor = 0
	s.input.Reset()
	s.input.EchoMode = textinput.EchoNormal
	switch s.step {
	case stepLang:
		s.cursor = slices.Index(i18n.Langs(), s.cfg.Language)
	case stepProvider:
		s.cursor = max(0, slices.Index(s.providerOpts(), s.cfg.ActiveProvider))
	case stepKey:
		s.input.EchoMode = textinput.EchoPassword
		s.input.Placeholder = s.cfg.Active().APIKeyEnv
		s.input.Focus()
	case stepCustom:
		var custom []string
		for _, r := range s.cfg.Releases {
			if !slices.Contains(Presets, r) {
				custom = append(custom, r)
			}
		}
		s.input.SetValue(strings.Join(custom, ", "))
		s.input.Placeholder = "owner/repo, owner/repo"
		s.input.Focus()
	case stepCity:
		s.input.SetValue(s.cfg.Location.City)
		s.input.Focus()
	}
}

func (s Setup) View() string {
	var b strings.Builder
	b.WriteString(styleApp.Render(s.t("setup.title")) + styleDim.Render(fmt.Sprintf("  [%d/%d]", s.step+1, stepCount)) + "\n\n")
	list := func(opts []string, label func(string) string, mark func(string) string) {
		for i, o := range opts {
			cur := "  "
			if i == s.cursor {
				cur = "▶ "
			}
			line := cur + mark(o) + label(o)
			if i == s.cursor {
				line = styleSel.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	none := func(string) string { return "" }
	switch s.step {
	case stepLang:
		b.WriteString(s.t("setup.lang") + "\n\n")
		list(i18n.Langs(), func(l string) string { return i18n.T(l, "lang.name") }, none)
	case stepProvider:
		b.WriteString(s.t("setup.provider") + "\n\n")
		list(s.providerOpts(), func(n string) string {
			if n == "" {
				return s.t("setup.provider_none")
			}
			return n
		}, none)
	case stepKey:
		b.WriteString(s.t("setup.key") + "\n" + styleDim.Render(s.t("setup.key_hint")) + "\n\n" + s.input.View() + "\n")
	case stepReleases:
		b.WriteString(s.t("setup.releases") + "\n\n")
		list(Presets, func(r string) string { return r }, func(r string) string {
			if s.picked[r] {
				return "[x] "
			}
			return "[ ] "
		})
	case stepCustom:
		b.WriteString(s.t("setup.custom_repo") + "\n\n" + s.input.View() + "\n")
	case stepCity:
		b.WriteString(s.t("setup.city") + "\n\n" + s.input.View() + "\n")
		if s.busy {
			b.WriteString("\n" + styleDim.Render(s.t("setup.city_searching")) + "\n")
		}
	}
	if s.err != "" {
		b.WriteString("\n" + styleErr.Render(s.err) + "\n")
	}
	b.WriteString("\n" + styleDim.Render(s.t("setup.nav")))
	return b.String()
}
