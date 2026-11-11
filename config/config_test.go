package config

import (
	"os"
	"slices"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GROQ_API_KEY", "k")

	c, err := Load()
	if err != nil || c.Language != "uk" || c.TranslationProvider != "auto" {
		t.Fatalf("defaults: %+v %v", c, err)
	}

	c.Language = "en"
	c.ActiveProvider = "groq"
	c.Releases = []string{"nodejs/node"}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(Path()); st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", st.Mode().Perm())
	}

	c2, err := Load()
	if err != nil || c2.Language != "en" || c2.Releases[0] != "nodejs/node" || c2.Active().Name != "groq" {
		t.Fatalf("reload: %+v %v", c2, err)
	}
}

func TestFixInvalid(t *testing.T) {
	c := &Config{Language: "ru", TranslationProvider: "x", ActiveProvider: "missing"}
	c.fix()
	if c.Language != "uk" || c.TranslationProvider != "auto" || c.ActiveProvider != "" || c.RefreshSec != 600 {
		t.Fatalf("%+v", c)
	}
	// picked in setup without a key and no env: back to FREE
	t.Setenv("GROQ_API_KEY", "")
	c = Default()
	c.ActiveProvider = "groq"
	c.fix()
	if c.ActiveProvider != "" {
		t.Fatal("provider without key must be dropped")
	}
}

func TestNextProvider(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "k")
	t.Setenv("GEMINI_API_KEY", "")
	c := Default()
	var got []string
	for range 4 {
		c.NextProvider()
		got = append(got, c.ActiveProvider)
	}
	// gemini skipped: no key
	if want := []string{"groq", "ollama", "", "groq"}; !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}
