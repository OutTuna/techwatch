// Package i18n holds UI strings. Adding a language = adding one file that calls register.
package i18n

import (
	"slices"
	"sort"
)

var dicts = map[string]map[string]string{}

func register(lang string, d map[string]string) { dicts[lang] = d }

func Langs() []string {
	l := make([]string, 0, len(dicts))
	for k := range dicts {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}

// T falls back to English, then to the key itself.
func T(lang, key string) string {
	if s, ok := dicts[lang][key]; ok {
		return s
	}
	if s, ok := dicts["en"][key]; ok {
		return s
	}
	return key
}

func Next(lang string) string {
	l := Langs()
	return l[(slices.Index(l, lang)+1)%len(l)]
}
