# techwatch

[Українською](README.uk.md)

Terminal dashboard for tech and security news: hacks and leaks, CVEs, software releases, tech news and weather. Headlines are translated to your language (Ukrainian or English) for free. If you add an LLM key, translation gets better and you also get a daily AI digest.

Inspired by / partly based on [lajosdeme/watchtower](https://github.com/lajosdeme/watchtower).

## Install

Requires Go 1.25+.

```bash
go install github.com/OutTuna/techwatch@latest
```

Or from source:

```bash
git clone https://github.com/OutTuna/techwatch && cd techwatch && go build -o techwatch . && ./techwatch
```

On first run a short setup opens: language, LLM provider (optional), API key, releases to track, city for weather. Run it again any time with `techwatch setup`.

## Modes

| Mode | Needs | Translation | AI digest |
|------|-------|-------------|-----------|
| FREE (default) | nothing | Google Translate (unofficial endpoint, ~1.4 req/s) | hint how to enable |
| LLM | key or local Ollama | LLM in batches of 30 headlines | yes |

Fallback chain: LLM → (error / 429) Google → (error) original title. Feeds already in your language are not translated. Translations are cached for 7 days, so only new headlines are sent.

`translation_provider`: `auto` (LLM if a provider is active, else Google), `google`, `llm`, `none`.

### Free LLM keys

- **Groq**: sign in at [console.groq.com/keys](https://console.groq.com/keys), create a key. Put it in setup or `export GROQ_API_KEY=...`.
- **Gemini**: [aistudio.google.com/apikey](https://aistudio.google.com/apikey), create a key. Put it in setup or `export GEMINI_API_KEY=...`.
- **Ollama** (local, no key): install from [ollama.com](https://ollama.com), run `ollama pull llama3.2`, choose `ollama` in setup.

Any OpenAI-compatible API works: add it to `providers` with `type: openai` and its `base_url`. Press `P` to switch between providers that have a key (and back to FREE).

## Keys

| Key | Action |
|-----|--------|
| `tab` / `shift+tab`, `1`–`6` | switch tab |
| `j` / `k` | down / up |
| `g` / `G` | top / bottom |
| `d` / `u` | page down / up |
| `enter` | open in browser |
| `r` | refresh |
| `f` | filter: category on Tech, source on other tabs |
| `L` | switch language (saved) |
| `P` | switch LLM provider (saved) |
| `q` | quit |

## Tabs and sources

- **Overview**: top critical items, latest releases, AI digest.
- **Security**: BleepingComputer, The Hacker News, Krebs on Security, The Record, SecurityWeek.
- **CVE**: CISA KEV (last 30 days, always CRITICAL), GitHub Security Advisories, Microsoft MSRC, Chrome Releases.
- **Releases**: `github.com/<owner>/<repo>/releases.atom` for repos from config.
- **Tech**: Ars Technica, The Verge, Hacker News, DOU, dev.ua, ITC.ua.
- **Weather**: Open-Meteo, current conditions and 7-day forecast.

Headlines are classified by the original title (en + uk keywords): CRITICAL (zero-day, actively exploited, RCE, CVSS 9–10, supply chain attack…), HIGH (breach, ransomware, leak, backdoor, privilege escalation, exploit), MEDIUM (vulnerability, patch, malware, phishing, botnet, DDoS), LOW (release, update, version, beta). CVE IDs are shown as badges.

## Config

`~/.config/techwatch/config.yaml` (written with mode 0600, it may contain keys). Setup, `L` and `P` rewrite this file, so comments you add there are lost.

```yaml
language: uk                # uk | en
show_original: true         # original title in gray under the translation
translation_provider: auto  # auto | google | llm | none
active_provider: ""         # "" = FREE, or a name from providers
providers:
  - name: groq
    type: openai
    base_url: https://api.groq.com/openai/v1
    model: llama-3.3-70b-versatile
    api_key_env: GROQ_API_KEY   # or api_key: gsk_...
  - name: gemini
    type: gemini
    base_url: https://generativelanguage.googleapis.com/v1beta
    model: gemini-2.5-flash
    api_key_env: GEMINI_API_KEY
  - name: ollama
    type: openai
    base_url: http://localhost:11434/v1
    model: llama3.2
releases:
  - torvalds/linux
  - golang/go
extra_feeds:
  - name: My blog
    url: https://example.com/feed.xml
    category: security          # security | vulns | releases | tech | local
    lang: en                    # feeds in your language are not translated
disabled_categories: []         # e.g. [local, tech]
location:
  city: Kyiv
  country: UA
  latitude: 50.4501
  longitude: 30.5234
temp_unit: celsius              # celsius | fahrenheit
refresh_seconds: 600
digest_cache_minutes: 60
cache_max_entries: 5000         # translation cache size
```

Other files in the same folder: `translations.json` (translation cache), `digest.json` (last digest).

## Adding a language

Copy `i18n/en.go` to `i18n/<code>.go`, change the code in `register` and translate the values. `go test ./i18n` checks that all keys are present.

## Development

```bash
go build ./... && go vet ./... && go test ./...
```

Network checks against real sources: `TECHWATCH_LIVE=1 go test ./... -run Live -v`.

## License

MIT, see [LICENSE](LICENSE). Watchtower's MIT license is in [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
