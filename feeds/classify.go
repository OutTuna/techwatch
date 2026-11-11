package feeds

import (
	"regexp"
	"slices"
	"strings"
)

type Level int

const (
	Info Level = iota
	Low
	Medium
	High
	Critical
)

func (l Level) String() string {
	return [...]string{"INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"}[l]
}

// Terms match from a word start, so stems like "вразлив" catch all word forms.
// \b is ASCII-only in RE2, hence the explicit boundary.
func tier(terms ...string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(?:` + strings.Join(terms, "|") + `)`)
}

const end = `(?:[^\p{L}\p{N}]|$)`

var tiers = []struct {
	level Level
	re    *regexp.Regexp
}{
	{Critical, tier(
		`zero[- ]day`, `0[- ]?day`, `actively exploited`, `in the wild`, `rce`+end, `remote code execution`,
		`cvss(?:v\d)?[^\d]{0,15}(?:9|10)(?:\.\d)?`+end, `wormable`, `supply[- ]chain attack`,
		`нульового дня`, `активно експлуат`, `віддален\p{L}* виконанн\p{L}* коду`, `атак\p{L}* на ланцюг\p{L}* постачання`,
	)},
	{High, tier(
		`breach`, `ransomware`, `data leak`, `leaked data`, `backdoor`, `privilege escalation`, `elevation of privilege`, `exploit`,
		`злам`, `витік\p{L}* даних`, `вимагач`, `бекдор`, `підвищенн\p{L}* привілеїв`, `експлойт`,
	)},
	{Medium, tier(
		`vulnerabilit`, `patch`, `malware`, `phishing`, `botnet`, `ddos`, `denial of service`,
		`вразлив`, `патч`, `шкідлив`, `фішинг`, `ботнет`,
	)},
	{Low, tier(
		`release`, `update`, `version`, `beta`, `v?\d+\.\d+`,
		`реліз`, `оновленн`, `верс`, `бета`,
	)},
}

func Classify(title string) Level {
	for _, t := range tiers {
		if t.re.MatchString(title) {
			return t.level
		}
	}
	return Info
}

var cveRe = regexp.MustCompile(`(?i)\bCVE-\d{4}-\d{4,}\b`)

func ExtractCVEs(texts ...string) []string {
	var out []string
	for _, t := range texts {
		for _, c := range cveRe.FindAllString(t, -1) {
			if c = strings.ToUpper(c); !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
}
