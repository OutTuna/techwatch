package i18n

import "testing"

func TestSameKeys(t *testing.T) {
	en := dicts["en"]
	for _, lang := range Langs() {
		d := dicts[lang]
		for k := range en {
			if _, ok := d[k]; !ok {
				t.Errorf("%s: missing %q", lang, k)
			}
		}
		for k := range d {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: extra %q", lang, k)
			}
		}
	}
}

func TestTAndNext(t *testing.T) {
	if T("uk", "tab.weather") != "Погода" || T("xx", "tab.weather") != "Weather" || T("en", "nope") != "nope" {
		t.Fatal("bad lookup")
	}
	if Next("en") != "uk" || Next("uk") != "en" {
		t.Fatal("bad cycle")
	}
}
