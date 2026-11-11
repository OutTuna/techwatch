package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/OutTuna/techwatch/i18n"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Feed struct {
	Name     string `mapstructure:"name" yaml:"name"`
	URL      string `mapstructure:"url" yaml:"url"`
	Category string `mapstructure:"category" yaml:"category"`
	Lang     string `mapstructure:"lang" yaml:"lang"`
}

// Provider is an LLM endpoint. Type is "openai" (any compatible API, incl. Groq and Ollama) or "gemini".
type Provider struct {
	Name      string `mapstructure:"name" yaml:"name"`
	Type      string `mapstructure:"type" yaml:"type"`
	BaseURL   string `mapstructure:"base_url" yaml:"base_url"`
	Model     string `mapstructure:"model" yaml:"model"`
	APIKey    string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	APIKeyEnv string `mapstructure:"api_key_env" yaml:"api_key_env,omitempty"`
}

func (p Provider) Key() string {
	if p.APIKey != "" {
		return p.APIKey
	}
	if p.APIKeyEnv != "" {
		return os.Getenv(p.APIKeyEnv)
	}
	return ""
}

// Usable: has a key, or is local and needs none.
func (p Provider) Usable() bool {
	return p.Key() != "" || (p.APIKey == "" && p.APIKeyEnv == "")
}

type Location struct {
	City      string  `mapstructure:"city" yaml:"city"`
	Country   string  `mapstructure:"country" yaml:"country"`
	Latitude  float64 `mapstructure:"latitude" yaml:"latitude"`
	Longitude float64 `mapstructure:"longitude" yaml:"longitude"`
}

type Config struct {
	Language            string     `mapstructure:"language" yaml:"language"`
	ShowOriginal        bool       `mapstructure:"show_original" yaml:"show_original"`
	TranslationProvider string     `mapstructure:"translation_provider" yaml:"translation_provider"`
	ActiveProvider      string     `mapstructure:"active_provider" yaml:"active_provider"`
	Providers           []Provider `mapstructure:"providers" yaml:"providers"`
	Releases            []string   `mapstructure:"releases" yaml:"releases"`
	ExtraFeeds          []Feed     `mapstructure:"extra_feeds" yaml:"extra_feeds"`
	DisabledCategories  []string   `mapstructure:"disabled_categories" yaml:"disabled_categories"`
	Location            Location   `mapstructure:"location" yaml:"location"`
	TempUnit            string     `mapstructure:"temp_unit" yaml:"temp_unit"`
	RefreshSec          int        `mapstructure:"refresh_seconds" yaml:"refresh_seconds"`
	DigestCacheMins     int        `mapstructure:"digest_cache_minutes" yaml:"digest_cache_minutes"`
	CacheMaxEntries     int        `mapstructure:"cache_max_entries" yaml:"cache_max_entries"`
}

var TranslationProviders = []string{"auto", "google", "llm", "none"}

func Default() *Config {
	return &Config{
		Language:            "uk",
		ShowOriginal:        true,
		TranslationProvider: "auto",
		Providers: []Provider{
			{Name: "groq", Type: "openai", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", APIKeyEnv: "GROQ_API_KEY"},
			{Name: "gemini", Type: "gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-2.5-flash", APIKeyEnv: "GEMINI_API_KEY"},
			{Name: "ollama", Type: "openai", BaseURL: "http://localhost:11434/v1", Model: "llama3.2"},
		},
		Releases:        []string{"torvalds/linux", "golang/go"},
		Location:        Location{City: "Kyiv", Country: "UA", Latitude: 50.4501, Longitude: 30.5234},
		TempUnit:        "celsius",
		RefreshSec:      600,
		DigestCacheMins: 60,
		CacheMaxEntries: 5000,
	}
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "techwatch")
}

func Path() string { return filepath.Join(Dir(), "config.yaml") }

func Exists() bool {
	_, err := os.Stat(Path())
	return err == nil
}

// Load returns defaults when there is no config file yet.
func Load() (*Config, error) {
	cfg := Default()
	if !Exists() {
		return cfg, nil
	}
	v := viper.New()
	v.SetConfigFile(Path())
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.fix()
	return cfg, nil
}

func (c *Config) fix() {
	d := Default()
	if !slices.Contains(i18n.Langs(), c.Language) {
		c.Language = d.Language
	}
	if !slices.Contains(TranslationProviders, c.TranslationProvider) {
		c.TranslationProvider = d.TranslationProvider
	}
	if c.RefreshSec < 60 {
		c.RefreshSec = d.RefreshSec
	}
	if c.DigestCacheMins <= 0 {
		c.DigestCacheMins = d.DigestCacheMins
	}
	if c.CacheMaxEntries <= 0 {
		c.CacheMaxEntries = d.CacheMaxEntries
	}
	if c.TempUnit != "fahrenheit" {
		c.TempUnit = "celsius"
	}
	if p := c.Active(); p == nil || !p.Usable() {
		c.ActiveProvider = ""
	}
}

func Save(c *Config) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return WriteFile(Path(), data)
}

// WriteFile writes atomically with 0600: files here may hold api keys.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Active returns the selected LLM provider, or nil in FREE mode.
func (c *Config) Active() *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == c.ActiveProvider {
			return &c.Providers[i]
		}
	}
	return nil
}

// NextProvider cycles: none -> each usable provider -> none.
func (c *Config) NextProvider() {
	names := []string{""}
	for _, p := range c.Providers {
		if p.Usable() {
			names = append(names, p.Name)
		}
	}
	i := slices.Index(names, c.ActiveProvider)
	c.ActiveProvider = names[(i+1)%len(names)]
}

func (c *Config) CategoryEnabled(cat string) bool {
	return !slices.Contains(c.DisabledCategories, cat)
}
