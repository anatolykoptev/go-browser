# Twitter Login Research Log

> Полная хронология исследований автоматического логина в Twitter через headless/headed Chrome.
> Проект: go-browser + CloakBrowser + go-social.

---

## Предыстория

**Цель**: Автоматический логин в Twitter аккаунты для go-social (управление соцсетями).
**go-social** (:8905) — централизованный менеджер аккаунтов: PostgreSQL + Redis, AES-256 encryption, pool rotation, REST API. Потребители: go-hully, ox-browser, vaelor.

### Этап 0: ox-browser (Rust + chromiumoxide) — до 2026-03-27

**ox-browser** — Rust сервис с `chromiumoxide` + `wreq` для HTTP/Chrome.

**Что было сделано (10+ коммитов):**
- stealth.js (9 модулей): screen dims, webdriver, plugins, canvas fingerprint
- SessionPool, get/set/destroy cookies
- InsertTextParams CDP typing (Playwright approach)
- nativeInputValueSetter + _valueTracker для React
- API login (`api_login.rs` + `api_flow.rs`) — twikit-style HTTP flow
- Dual stealth profiles (full/lite для CloakBrowser)

**Ключевые находки ox-browser эпохи:**
- `DispatchKeyEvent` типирует текст в DOM но НЕ обновляет React state
- `nativeInputValueSetter` + `_valueTracker.setValue('')` + `Event('input')` — правильный React bypass
- `element.click()` через CDP mouse dispatch работает для React кнопок
- Guest token работает для активации, но НЕ для TweetDetail (404)

**Блокеры:**
- chromiumoxide `.arg()` не передавал `--fingerprint-*` флаги CloakBrowser → navigator.platform = `Linux x86_64` вместо `Win32`
- Решение: CloakBrowser как Docker sidecar + `Browser::connect("ws://cloakbrowser:9222")`
- Castle.io зависал (spinner) или возвращал 399

**Промежуточное решение — Playwright:**
- Рассматривали Playwright MCP (`@playwright/mcp`) внутри ox-browser контейнера
- Вывод: Playwright имеет battle-tested stealth, но snap proxy несовместимость + CORS блокировали

**Решение: миграция на Go.** go-rod (Grade A, 18K stars) + CloakBrowser sidecar.

### CloakBrowser Sidecar (2026-03-27)

- Docker sidecar `cloakhq/cloakbrowser:latest` (:9222)
- WS URL discovery через `/json/version`
- Host header override для Chrome DevTools
- 33 C++ патча: canvas, webgl, audio, screen, navigator, fonts, locale
- reCAPTCHA 0.9, Turnstile PASS

### Castle Solver — botwitter.com (2026-03-27)

- `castle.botwitter.com/generate-token` REST API
- Rate limit: 2 запроса/час per IP (free tier)
- Обход: Webshare proxy rotation (215K residential IPs, каждый порт = свежий IP)
- XHR interceptor в браузере инжектил castle_token в task.json `settings_list`
- Токены генерировались (2246-3008 chars), но Twitter отвечал 399

**Текущий стек**: go-browser (Go + go-rod) + CloakBrowser (patched Chromium 145, 33 C++ патча) + go-social (workflow engine).

---

## Хронология исследований (2026-03-27 — 2026-03-28)

### Этап 1: Миграция на go-browser

**Проблема**: CloakBrowser крашился после первого запроса (ExitCode=0).
**Причина**: Headless Chrome закрывается когда все BrowserContext'ы disposed.
**Решение**: Keepalive context — фоновый контекст с about:blank страницей. Коммит `1e86ea9`.

**Проблема**: Proxy auth не работал (ERR_NO_SUPPORTED_PROXIES).
**Причина**: Chrome CDP ProxyServer не поддерживает user:pass@host формат.
**Решение**: `parseProxy()` разделяет credentials, `setupProxyAuth()` обрабатывает 407 через CDP Fetch.authRequired. Коммит `9cb1014`.

**Проблема**: Fetch domain блокировал страничные XHR (network_error на task.json).
**Причина**: FetchRequestPaused handler блокировал/задерживал запросы страницы.
**Решение**: Обработка RequestPaused и AuthRequired в goroutines (`go func(){...}()`).

### Этап 2: Castle.io — первое прохождение

**Проблема**: Castle.io зависал бесконечно (spinner на login странице).
**Гипотеза**: Castle SDK не получает достаточно fingerprint/behavioral данных.

**Фиксы (серия коммитов)**:
| Фикс | Коммит | Эффект |
|-------|--------|--------|
| `--fingerprint-platform=macos` | `1882a46` | GPU/platform consistency |
| WebGL через SwiftShader ANGLE | `423d598` | Castle проверяет WebGL renderer |
| `--window-size=1920,1080` | `423d598` | Screen/window совпадение |
| `navigator.webdriver=false` (не undefined) | `939eb8c` | Chrome спецификация |
| `chrome.csi/loadTimes/app` stubs | `939eb8c` | Castle проверяет эти объекты |
| `navigator.userAgentData` macOS | `af14763` | Client Hints |
| `navigator.mediaDevices` stub | `1e86ea9` | Headless detection |
| Модульные stealth профили | `5235648+` | Динамические per-request |

**Результат**: Castle.io начал генерировать токен (2246 chars). Но ответ — **399 "Could not log you in now"** вместо зависания.

### Этап 3: Исследование 399

**Гипотеза 1: IP reputation** (datacenter IP 192.9.243.148).
- Тест через Webshare residential proxy → 399.
- Тест через SSH tunnel с ноутбука (residential IP 216.77.50.8) → 399.
- **Вывод**: НЕ IP.

**Гипотеза 2: Castle token от botwitter.com несовместим**.
- botwitter.com castle_token (3008 chars) инжектировался через XHR interceptor.
- Token инжекция подтверждена: `{subtask_ids: ["LoginEnterUserIdentifierSSO"], injected: true, ct_len: 3008}`.
- Но 399 сохранялся.
- Проверка: Castle headers (X-Castle-Request-Token) НЕ отправляются — только body.
- **Вывод**: botwitter.com токен отклоняется Twitter'ом.

**Гипотеза 3: Castle token формат устарел**.
- Анализ d60/twitter_login (Python) — генерирует castle_token ЛОКАЛЬНО.
- Портировали генератор на Go (internal/castle/): XXTEA + fingerprint + behavioral data.
- 33 теста, cross-verified против Python — результаты идентичны.
- Python d60/twitter_login тоже получает 399.
- Python twikit тоже не работает.
- **Вывод**: ВСЕ open-source решения для Twitter login сломаны. Castle протокол обновился.

**Гипотеза 4: Browser + XHR interceptor конфликтует с Castle SDK**.
- Castle SDK в браузере генерирует свой токен.
- XHR interceptor подменяет его нашим → конфликт.
- Тест БЕЗ interceptor (Castle SDK сам генерирует) → 399.
- **Вывод**: НЕ конфликт interceptor'а. Castle SDK пропускает, но Twitter отклоняет.

### Этап 4: Глубокий аудит stealth системы

Полный аудит обнаружил **3 CRITICAL + 9 HIGH** проблем:

**CRITICAL:**
1. `Function.prototype.toString()` — все переопределённые getters возвращали arrow function source вместо `[native code]`.
2. Worker proxy ломался на Blob/data: URLs — Castle создаёт Worker из Blob, проверяет navigator внутри.
3. SwiftShader GPU != spoofed "Intel Iris" — GL capabilities не совпадали.

**HIGH:**
4. Профиль противоречив: `arm` arch + Intel GPU + 1920x1080@2x + platformVersion 14.5.0 ≠ UA 10_15_7.
5. `colorDepth: 30` невозможен для Intel Iris.
6. `InputInsertText` — текст без keyboard events.
7. `document.hasFocus()` = false в headless.
8. `Accept-Language` header не совпадал с `navigator.languages`.
9. `outerWidth/outerHeight` = 0 в headless.
10. Worker `userAgent` не патчился.
11. `chrome.csi()/loadTimes()` — идентичные timestamps.
12. Stealth markers (`__sp`, `__stealthProfile`) не удалялись.

**Все 12 проблем исправлены.**

### Этап 5: Headed Chrome (Xvfb)

**Идея**: Не headless, а настоящий Chrome с виртуальным дисплеем.
**Реализация**: Xvfb :99 + dbus-daemon в Docker контейнере CloakBrowser.
**Результат**: 8 detection vectors исчезли НАТИВНО без JS overrides:
- `document.hasFocus()` = true нативно
- `visibilityState` = visible нативно
- `speechSynthesis.getVoices()` = 9 голосов нативно
- `performance.memory` = реальные значения нативно
- CSS media queries (`hover: hover`, `pointer: fine`) нативно
- `innerWidth` ≠ `outerWidth` (реальный viewport)

**Ресурсы**: 284 MB RAM vs 160 MB headless (+77%). CPU = 0% idle.
**399**: сохранился.

### Этап 6: Phase 1 Quick Fixes

Реализованы:
- Worker `userAgent` patch
- `chrome.csi()/loadTimes()` — реалистичные timing из `performance.timing`
- `Accept-Language` header через CDP `Network.setExtraHTTPHeaders`
- `getBattery()` / `getGamepads()` stubs
- Localhost port scan protection (fetch + WebSocket блокировка)

**399**: сохранился.

### Этап 7: TLS/HTTP2 fingerprint верификация

Сравнение CloakBrowser vs реальный Chrome 145 через `tls.peet.ws/api/all`:

| Параметр | CloakBrowser | Реальный Chrome | Совпадает? |
|----------|:---:|:---:|:---:|
| JA4 | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | ✅ |
| Akamai HTTP/2 hash | `52d84b11737d980aef856699f885ca86` | `52d84b11737d980aef856699f885ca86` | ✅ |
| Peetprint hash | `1d4ffe9b0e34acac0bd883fa7f79d7b5` | `1d4ffe9b0e34acac0bd883fa7f79d7b5` | ✅ |
| Ciphers (16 штук) | идентичны | идентичны | ✅ |
| HTTP/2 SETTINGS | `1:65536;2:0;4:6291456;6:262144` | `1:65536;2:0;4:6291456;6:262144` | ✅ |
| User-Agent | Chrome/145.0.0.0 macOS | Chrome/145.0.0.0 macOS | ✅ |

**Вывод**: TLS и HTTP/2 fingerprint ИДЕНТИЧНЫ. НЕ причина 399.

### Этап 8: x-client-transaction-id

Портировали генератор из d60/twitter_login (342 строки Python) на Go:
- `txid.go` — верификационный ключ из HTML, animation key из SVG
- `cubic.go` — cubic bezier curve evaluation
- `txid_animate.go` — CSS animation math
- SHA256 + XOR + base64 encoding

Интегрировали в `client.go` — header добавляется автоматически к каждому API запросу.

**399**: сохранился.

### Этап 9: Тест всех аккаунтов

| Аккаунт | IP | Proxy | Результат |
|---------|-----|-------|-----------|
| piteronlinetv | datacenter | нет | 399 |
| piteronlinetv | Webshare residential | да | 399 |
| piteronlinetv | Mac SSH tunnel | да | 399 |
| anatolykoptev | datacenter | нет | 399 |
| anatolykoptev | Webshare residential | да | 399 |
| hullyday | datacenter | нет | 399 |
| EvalikFulvik | datacenter | нет | 399 |

**Ручной логин** с Mac (216.77.50.8) в реальном Chrome → **работает**.

---

## Что построено (артефакты)

### go-browser (~/src/go-browser/)
- 17 action types, session pool, humanize, stealth
- 6 stealth JS модулей (384 строки → stealth_complement.js)
- 3 fingerprint профиля (mac/win/linux Chrome 145)
- Headed Chrome через Xvfb + dbus
- 111 тестов
- Порт: 8906

### go-social castle token (~/src/go-social/internal/castle/)
- XXTEA encryption, fingerprint preset, behavioral encoding
- Go порт d60/twitter_login castle_token
- 33 теста, cross-verified vs Python

### go-social API login (~/src/go-social/internal/login/)
- HTTP client с go-stealth TLS
- ui_metrics solver (goja + MockDocument)
- Login flow state machine
- x-client-transaction-id generator
- 32 теста

### Документация
- `go-browser/docs/STEALTH-SPEC.md` — полная спецификация stealth
- `go-browser/docs/STEALTH-ROADMAP.md` — 5 фаз, detection vectors
- `go-browser/docs/RESEARCH-LOG.md` — этот документ

---

## Исключённые гипотезы (доказано что НЕ причина)

1. ❌ TLS fingerprint (JA3/JA4) — идентичен реальному Chrome
2. ❌ HTTP/2 SETTINGS frame — идентичен
3. ❌ JS fingerprint (20+ параметров) — все проходят
4. ❌ Headless detection — headed Chrome через Xvfb
5. ❌ Castle.io SDK — генерирует токен стабильно
6. ❌ Castle token формат — и Python и Go реализации дают 399
7. ❌ IP reputation — 399 на datacenter, residential, SSH tunnel
8. ❌ Аккаунт — все 4 аккаунта дают 399
9. ❌ XHR interceptor конфликт — 399 и без interceptor
10. ❌ x-client-transaction-id — реализован, 399 сохраняется
11. ❌ Accept-Language header — совпадает
12. ❌ Keyboard events — полный keyDown/char/keyUp

## Оставшиеся гипотезы

1. **CDP протокол детектируется на C++ уровне** — Chrome internals могут иметь side effects от CDP подключения, невидимые из JS. Например, дополнительные WebSocket соединения, изменённое поведение V8 при подключённом debugger.

2. **CloakBrowser C++ патчи оставляют следы** — 33 патча изменяют поведение Chromium. Даже если результат идентичен реальному Chrome для известных проверок, могут быть неизвестные side effects.

3. **Server-side ML модель** — Twitter может использовать ML для скоринга сессий по совокупности сотен сигналов, где каждый по отдельности "нормальный", но комбинация аномальна.

4. **Поведенческий анализ** — наш warmup (5 сек случайных движений) может отличаться от реального паттерна "человек открыл страницу логина". Реальный пользователь: смотрит на страницу → фокусируется на поле → кликает → печатает. Наш бот: случайные движения → сразу в поле.

5. **Временнóй профиль** — время между page load и первым действием, между вводом username и кликом Next. ML модель может знать "нормальное" распределение этих интервалов.

6. **Навигационная история** — реальный пользователь может приходить с google.com, из закладок, с другой страницы Twitter. Наш бот всегда заходит напрямую на /i/flow/login.

---

## Этап 10: API Hooking — что Twitter реально делает (2026-03-31)

### Инструмент

Построен `security_scan_browser` (go-wowa MCP tool) с pre-inject API hooking:
- go-browser `PreActions` — `eval_on_new_document` до navigation
- 17 перехваченных browser API: canvas (3), WebGL (3), audio (3), permissions, media devices, battery, WebRTC, WebSocket, fetch, XHR
- Хуки подменяют нативные методы, считают вызовы, логируют URL-ы

### Результат: x.com/i/flow/login

| API | Вызовы | Что делает |
|-----|--------|-----------|
| **Canvas** | **50** | Серия canvas probe-ов — рисует, читает pixel data, хеширует |
| **WebGL** | **12** | getParameter + getExtension — полный GPU/driver fingerprint |
| **Permissions** | **4** | query(camera/mic/notifications/geolocation) — capability profiling |
| **MediaDevices** | **1** | enumerateDevices — уникальный список камер/микрофонов |
| **Battery** | **1** | getBattery — level/charging adds entropy |
| **WebRTC** | **1** | createDataChannel — IP leak probing |
| **XHR** | **6** | user_flow.json, onboarding/task.json — session telemetry |

Итого: **69 API-вызовов** в первые 3 секунды загрузки login page. **Ни один** из этих сигналов не был виден через HTTP-only анализ.

### Ключевой вывод: почему 399

Все предыдущие этапы (1-9) фокусировались на TLS, HTTP/2, JS navigator, Castle token — то есть на **метаданных сессии**. Но Castle.io (встроенный в Twitter бандл, обфусцирован) на самом деле оценивает **browser API activity**:

```
Реальный браузер:                    Наш бот (HTTP API или CloakBrowser):
- Canvas: 50 calls → hash A          - Canvas: 0 calls (API) или hash B (CloakBrowser рандомизирует)
- WebGL: 12 calls → GPU X            - WebGL: 0 calls (API) или GPU "SwiftShader" (headless)
- Battery: level 0.87, charging       - Battery: 0 calls (API) или stub value
- Permissions: 4 queries              - Permissions: 0 calls
- MediaDevices: [Camera, Mic]         - MediaDevices: 0 calls или []
- WebRTC: DataChannel → local IP     - WebRTC: 0 calls
```

Castle.io ML видит: **нулевая browser API активность** при наличии castle_token → score ≈ 0 → 399.

Для CloakBrowser: **рандомизированные ответы** на canvas/WebGL (33 C++ патча) дают **разный hash каждый раз** → Castle.io видит нестабильный fingerprint → score низкий → 399.

### Переоценка исключённых гипотез

| # | Гипотеза | Старый статус | Новый статус |
|---|----------|--------------|-------------|
| 3 | Server-side ML | Оставшаяся | ✅ **ПОДТВЕРЖДЕНА** — 69 client-side сигналов для ML scoring |
| 4 | Поведенческий анализ | Оставшаяся | ✅ **ПОДТВЕРЖДЕНА** — canvas/WebGL calls = behavioral signal |
| 5 | Castle.io SDK | ❌ Исключена ("генерирует токен") | ⚠️ **ПЕРЕОЦЕНЕНА** — токен генерируется, но без API activity = пустой fingerprint |

### Что нужно для прохождения Castle.io

1. **Не блокировать Castle SDK** — пусть сам собирает canvas/WebGL/battery/etc.
2. **Консистентный canvas fingerprint** — CloakBrowser рандомизирует каждый вызов; нужен stable hash через все 50 calls
3. **Реалистичный WebGL** — SwiftShader GPU ≠ любой реальный GPU; нужен GPU spoofing с правильными extension capabilities
4. **Заполнить все API** — Battery, MediaDevices, Permissions, WebRTC должны возвращать реалистичные данные
5. **Timing** — дать Castle SDK 10-15 сек на сбор (наш warmup 3-5 сек был недостаточен)

---

## Рекомендации (обновлено 2026-03-31)

### Краткосрочно (работает сейчас)
**Cookie import** — залогиниться вручную, экспортировать `auth_token` + `ct0`, использовать через API. Refresh раз в 30+ дней. go-social уже поддерживает `login_with_cookies`.

### Среднесрочно
1. **Stable canvas fingerprint** — патч CloakBrowser чтобы canvas давал стабильный (не рандомный) hash per profile. Один профиль = один fingerprint навсегда.
2. **Realistic GPU spoofing** — вместо SwiftShader использовать GPU spoofing с правильным RENDERER/VENDOR string + matching extensions. CloakBrowser уже спуфит строку, но capabilities (WebGL parameters) должны совпадать с реальным GPU.
3. **Full API population** — дополнить stealth_complement.js: Battery (level/charging/chargingTime), MediaDevices (Camera+Mic list), Permissions (realistic states). Привязать к профилю.
4. **Castle warm-up time** — увеличить задержку до 10-15 сек перед первым действием на login page. Castle SDK должен успеть отправить telemetry.
5. **Мониторить d60/twitter_login** — когда починят Castle protocol, наш Go порт заработает автоматически (но API login без browser всё равно будет блокироваться).

### Долгосрочно
1. ✅ **Anti-bot detector tool** — **РЕАЛИЗОВАН** (`security_scan` + `auto_bypass`). 103 WAF signatures, 21 API hooks, 22 bypass profiles. Details: go-wowa ARCHITECTURE.md.
2. ✅ **Adaptive stealth profiles** — **РЕАЛИЗОВАН** (protection profiles в go-wowa `bypass_profile.go`). matchProfile() выбирает рецепт по обнаруженной защите.
3. **Behavioral ML** — обучить модель на записях реальных пользовательских сессий, воспроизводить паттерны.
4. **Device farm** — реальные устройства с реальными браузерами, управляемые через API.

---

## Этап 11: LinkedIn Login — успешный обход (2026-04-01)

### Контекст

В отличие от Twitter (Castle.io ML + 399), LinkedIn использует более простую защиту: Cloudflare Bot Management + fingerprinting (canvas, WebGL, navigator). Нет Castle.io. Основной барьер — TLS fingerprint binding cookies.

### go-wowa Auto-Bypass Engine

Построена полная система обнаружения и обхода в go-wowa (`chrome_interact` с `auto_bypass=true`):

**Фаза 0 — Evasion (до навигации):**
- `stealth_evasions.js` — 7 активных патчей (ported from puppeteer-extra-stealth):
  - `navigator.webdriver` → undefined через Proxy `has()` trap
  - `window.chrome.runtime` → fake объект
  - `navigator.plugins` → 3 реалистичных плагина
  - `permissions.query('notifications')` → 'denied'
  - iframe `contentWindow.chrome` → MutationObserver patch
  - `canPlayType` → correct codec responses
  - WebGL `UNMASKED_RENDERER` → NVIDIA вместо SwiftShader
- `stealth_canvas_noise.js` — deterministic LCG PRNG (±1px, ~2% pixels)

**Фаза 1 — Detection (после действий):**
- `security_hooks.js` — 21 API hook категорий (canvas, webgl, audio, rtc, battery, etc.)
- `security_probe.js` — globals, DOM signals, meta, cookies
- `MapSignalsToDetections()` → 25+ globals, 12 DOM signals, 20+ network patterns
- `classifyRisk()` → low/medium/high

**Фаза 2 — Profile Match:**
- `matchProfile()` — 22 профиля (CF, DataDome, PX, Akamai, Shape, Imperva, FingerprintJS, 8 CAPTCHA, IP Block)
- Приоритет: bot_detection(100) > waf(80) > captcha(60) > fingerprinting(40) > access_block(20)

**Фаза 3 — Bypass (если profile.CanAutoBypass):**
- Stealth profile rotation (win/mac/linux_chrome145)
- Proxy strategy (residential / rotate_port)
- Pre-sleep (CF: 8000ms)
- Canvas noise injection (per profile)
- `wait_for cookie` polling (_px3, datadome)

### LinkedIn Login — пошаговый результат

```
1. Navigate linkedin.com/login (proxy p.webshare.io:10030 = IP 154.192.119.80)
2. stealth_evasions.js инжектирован → headless signals скрыты
3. fill_form (#username + #password)
4. click submit (humanize=true, Bezier mouse path)
5. LinkedIn показал App Challenge (2FA через мобильное приложение)
6. sleep 60s → пользователь подтвердил в LinkedIn App
7. Redirect → linkedin.com/feed/ ✅
8. li_at cookie получен ✅
9. 12 fingerprinting hooks обнаружены но НЕ заблокировали
```

### Ключевое открытие: TLS Fingerprint Cookie Binding

LinkedIn привязывает `li_at` к JA3/JA4 TLS fingerprint:

| Клиент | TLS Stack | JA3 | Cookie принят? |
|--------|-----------|-----|:-:|
| CloakBrowser (Chrome 145) | BoringSSL | Chrome JA3 | ✅ |
| curl | libcurl/OpenSSL | curl JA3 | ❌ (`li_at=delete me`) |
| go-stealth (utls) | utls | utls JA3 | ❌ (302 redirect) |
| ox-browser (wreq) | BoringSSL | wreq JA3 | ❌ (не тестировали, но вероятно ≠ Chrome) |

**Вывод:** Cookie работает ТОЛЬКО в том TLS клиенте, в котором был получен.

**Последствие:** Нельзя получить cookies через Chrome и использовать через go-stealth HTTP клиент. Login и API вызовы ДОЛЖНЫ использовать один TLS stack.

**Решение:** `go-linkedin.Login()` через go-stealth HTTP (не Chrome). Тогда cookies будут валидны для go-linkedin API вызовов через тот же go-stealth TLS.

### Сравнение Twitter vs LinkedIn

| Параметр | Twitter (x.com) | LinkedIn |
|----------|:---:|:---:|
| Anti-bot | Castle.io (ML) | Cloudflare Bot Management |
| Fingerprinting | 69 API calls в 3 сек | 12 API hooks (canvas, webgl, screen, navigator) |
| TLS binding | Не обнаружено | ✅ li_at привязан к JA3 |
| Headless detection | Блокирует (399) | Не блокирует (с evasions) |
| CDP detection | Runtime.enable | Не обнаружено |
| 2FA | SMS/TOTP/Email | App Challenge (push в мобильное приложение) |
| Chrome login | ❌ 399 (Castle ML) | ✅ Работает с evasions |
| HTTP login | ❌ 399 (все решения) | ❌ Не реализован (TODO) |
| Статус | Все open-source сломаны | Chrome ✅, HTTP TODO |

### Артефакты сессии 2026-04-01

| Файл | Repo | Назначение |
|------|------|-----------|
| `stealth_evasions.js` | go-wowa | 7 активных evasion патчей |
| `stealth_canvas_noise.js` | go-wowa | Deterministic canvas noise |
| `bypass_profile.go` | go-wowa | 22 protection profiles |
| `interact_detect.go` | go-wowa | quickDetect() + matchProfile() |
| `interact_bypass.go` | go-wowa | Profile-driven bypass engine |
| `linkedin-login.json` | go-social | go-workflow template для Chrome login |
| `linkedin_pool.go` | go-job | Lazy client с auto-refresh |
| `ARCHITECTURE.md` | go-wowa | Full architecture documentation |

### План: TLS Refactor

Файл: `docs/superpowers/plans/2026-04-01-linkedin-tls-refactor.md`

1. `go-linkedin.Login(email, password)` через go-stealth HTTP
2. go-social `api_linkedin_login` tool
3. Auto-relogin при 302/403
4. go-job report failure → trigger relogin
5. E2E verification
