# Twitter Login Research Log

> Полная хронология исследований автоматического логина в Twitter через headless/headed Chrome.
> Проект: go-browser + CloakBrowser + go-social.

---

## Предыстория

**Цель**: Автоматический логин в Twitter аккаунты для go-social (управление соцсетями).

**Предыдущий подход** (ox-browser, Rust + chromiumoxide): работал частично — `enter_username` проходил, но Castle.io зависал на неопределённое время. chromiumoxide заброшен автором → решили мигрировать на Go.

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

## Рекомендации

### Краткосрочно (работает сейчас)
**Cookie import** — залогиниться вручную, экспортировать `auth_token` + `ct0`, использовать через API. Refresh раз в 30+ дней. go-social уже поддерживает `login_with_cookies`.

### Среднесрочно
1. **Мониторить d60/twitter_login** — когда починят Castle protocol, наш Go порт заработает автоматически.
2. **BotBrowser** (PRO tier, ~$200/мес) — коммерческий antidetect Chrome с гарантией прохождения Castle. Можно подключить вместо CloakBrowser.
3. **Playwright + реальный Chrome** — запустить не CloakBrowser, а обычный Chrome через Playwright, без CDP stealth patches. Может дать другой результат.

### Долгосрочно
1. **Anti-bot detector tool** — определять какие защиты стоят на сайте, адаптировать stealth.
2. **Behavioral ML** — обучить модель на записях реальных пользовательских сессий, воспроизводить паттерны.
3. **Device farm** — реальные устройства с реальными браузерами, управляемые через API.
