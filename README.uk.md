# techwatch

[English](README.md)

Термінальний дашборд tech- і security-новин: зломи та витоки, CVE, релізи програм, технологічні новини й погода. Заголовки безкоштовно перекладаються вашою мовою (українська або англійська). Якщо додати ключ LLM, переклад стане кращим і з'явиться щоденний AI-дайджест.

Inspired by / partly based on [lajosdeme/watchtower](https://github.com/lajosdeme/watchtower).

## Встановлення

Потрібен Go 1.25+.

```bash
go install github.com/OutTuna/techwatch@latest
```

Або з вихідного коду:

```bash
git clone https://github.com/OutTuna/techwatch && cd techwatch && go build -o techwatch . && ./techwatch
```

Під час першого запуску відкриється коротке налаштування: мова, LLM-провайдер (необов'язково), API-ключ, релізи для відстеження, місто для погоди. Запустити його знову: `techwatch setup`.

## Режими

| Режим | Потрібно | Переклад | AI-дайджест |
|-------|----------|----------|-------------|
| FREE (типово) | нічого | Google Translate (неофіційний endpoint, ~1,4 запиту/с) | підказка, як увімкнути |
| LLM | ключ або локальна Ollama | LLM пачками по 30 заголовків | так |

Ланцюжок fallback: LLM → (помилка / 429) Google → (помилка) оригінал. Стрічки вашою мовою не перекладаються. Переклади кешуються на 7 днів, тож надсилаються лише нові заголовки.

`translation_provider`: `auto` (LLM, якщо провайдер активний, інакше Google), `google`, `llm`, `none`.

### Безкоштовні ключі LLM

- **Groq**: увійдіть на [console.groq.com/keys](https://console.groq.com/keys), створіть ключ. Вкажіть його в setup або `export GROQ_API_KEY=...`.
- **Gemini**: [aistudio.google.com/apikey](https://aistudio.google.com/apikey), створіть ключ. Вкажіть його в setup або `export GEMINI_API_KEY=...`.
- **Ollama** (локально, без ключа): встановіть з [ollama.com](https://ollama.com), виконайте `ollama pull llama3.2`, оберіть `ollama` у setup.

Підходить будь-який OpenAI-сумісний API: додайте його в `providers` з `type: openai` і своїм `base_url`. `P` перемикає між провайдерами, що мають ключ (і назад у FREE).

## Клавіші

| Клавіша | Дія |
|---------|-----|
| `tab` / `shift+tab`, `1`–`6` | вкладки |
| `j` / `k` | вниз / вгору |
| `g` / `G` | на початок / в кінець |
| `d` / `u` | сторінка вниз / вгору |
| `enter` | відкрити в браузері |
| `o` | читати статтю в бічній панелі з перекладом (`j/k` прокрутка, `esc` закрити) |
| `r` | оновити |
| `f` | фільтр: категорія на «Технології», джерело на інших вкладках |
| `L` | змінити мову (зберігається) |
| `P` | змінити LLM-провайдер (зберігається) |
| `q` | вихід |

## Вкладки та джерела

- **Огляд**: найкритичніше, останні релізи, AI-дайджест.
- **Безпека**: BleepingComputer, The Hacker News, Krebs on Security, The Record, SecurityWeek.
- **CVE**: CISA KEV (за 30 днів, завжди CRITICAL), GitHub Security Advisories, Microsoft MSRC, Chrome Releases.
- **Релізи**: `github.com/<owner>/<repo>/releases.atom` для репозиторіїв з конфігу.
- **Технології**: Ars Technica, The Verge, Hacker News, DOU, dev.ua, ITC.ua.
- **Погода**: Open-Meteo, поточна погода й прогноз на 7 днів.

Класифікація за оригінальним заголовком (ключові слова en + uk): CRITICAL (zero-day, активна експлуатація, RCE, CVSS 9–10, атака на ланцюг постачання…), HIGH (злам, ransomware, витік, бекдор, підвищення привілеїв, експлойт), MEDIUM (вразливість, патч, шкідливе ПЗ, фішинг, ботнет, DDoS), LOW (реліз, оновлення, версія, бета). CVE показуються бейджами.

## Конфіг

`~/.config/techwatch/config.yaml` (записується з правами 0600, бо може містити ключі). Setup, `L` і `P` перезаписують файл, тож ваші коментарі в ньому зникнуть. Повний приклад з коментарями — у [README.md](README.md#config). Основне:

```yaml
language: uk                # uk | en
show_original: true         # оригінал сірим під перекладом
translation_provider: auto  # auto | google | llm | none
active_provider: ""         # "" = FREE, або name з providers
releases: [torvalds/linux, golang/go]
extra_feeds:
  - name: Мій блог
    url: https://example.com/feed.xml
    category: security
    lang: uk
disabled_categories: []     # напр. [local, tech]
```

Місто змінюйте через `techwatch setup`: координати для погоди шукаються автоматично.

Інші файли в тій самій теці: `translations.json` (кеш перекладів), `digest.json` (останній дайджест).

## Нова мова

Скопіюйте `i18n/en.go` у `i18n/<код>.go`, змініть код у `register` і перекладіть значення. `go test ./i18n` перевіряє, що всі ключі на місці.

## Розробка

```bash
go build ./... && go vet ./... && go test ./...
```

Перевірка з реальними джерелами: `TECHWATCH_LIVE=1 go test ./... -run Live -v`.

## Ліцензія

MIT, див. [LICENSE](LICENSE). MIT-ліцензія watchtower — у [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
