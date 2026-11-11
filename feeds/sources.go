package feeds

import (
	"regexp"
	"slices"

	"github.com/OutTuna/techwatch/config"
)

const (
	KEVURL  = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	GHSAURL = "https://api.github.com/advisories?type=reviewed&per_page=50"
)

// Dark Reading dropped: Cloudflare returns 403 to non-browser clients.
var Builtin = []config.Feed{
	{Name: "BleepingComputer", URL: "https://www.bleepingcomputer.com/feed/", Category: "security", Lang: "en"},
	{Name: "The Hacker News", URL: "https://feeds.feedburner.com/TheHackersNews", Category: "security", Lang: "en"},
	{Name: "Krebs on Security", URL: "https://krebsonsecurity.com/feed/", Category: "security", Lang: "en"},
	{Name: "The Record", URL: "https://therecord.media/feed", Category: "security", Lang: "en"},
	{Name: "SecurityWeek", URL: "https://www.securityweek.com/feed/", Category: "security", Lang: "en"},

	{Name: "CISA KEV", URL: KEVURL, Category: "vulns", Lang: "en"},
	{Name: "GitHub Advisories", URL: GHSAURL, Category: "vulns", Lang: "en"},
	{Name: "Microsoft MSRC", URL: "https://api.msrc.microsoft.com/update-guide/rss", Category: "vulns", Lang: "en"},
	{Name: "Chrome Releases", URL: "https://chromereleases.googleblog.com/feeds/posts/default", Category: "vulns", Lang: "en"},

	{Name: "Ars Technica", URL: "https://feeds.arstechnica.com/arstechnica/index", Category: "tech", Lang: "en"},
	{Name: "The Verge", URL: "https://www.theverge.com/rss/index.xml", Category: "tech", Lang: "en"},
	{Name: "Hacker News", URL: "https://hnrss.org/frontpage", Category: "tech", Lang: "en"},

	{Name: "DOU", URL: "https://dou.ua/feed/", Category: "local", Lang: "uk"},
	{Name: "dev.ua", URL: "https://dev.ua/rss", Category: "local", Lang: "uk"},
	{Name: "ITC.ua", URL: "https://itc.ua/feed/", Category: "local", Lang: "uk"},
}

var Categories = []string{"security", "vulns", "releases", "tech", "local"}

var repoRe = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

func ValidRepo(s string) bool { return repoRe.MatchString(s) }

// Sources = builtin + releases + extra feeds, minus disabled categories.
func Sources(cfg *config.Config) []config.Feed {
	all := slices.Clone(Builtin)
	for _, r := range cfg.Releases {
		if repoRe.MatchString(r) {
			all = append(all, config.Feed{Name: r, URL: "https://github.com/" + r + "/releases.atom", Category: "releases", Lang: "en"})
		}
	}
	for _, f := range cfg.ExtraFeeds {
		if f.Lang == "" {
			f.Lang = "en"
		}
		if !slices.Contains(Categories, f.Category) {
			f.Category = "tech"
		}
		all = append(all, f)
	}
	var out []config.Feed
	for _, f := range all {
		if cfg.CategoryEnabled(f.Category) {
			out = append(out, f)
		}
	}
	return out
}
