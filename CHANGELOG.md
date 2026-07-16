# Changelog

## [0.20.2](https://github.com/anatolykoptev/go-browser/compare/v0.20.1...v0.20.2) (2026-07-16)


### Added

* CDP fault injection mock for testing recovery paths without Chrome ([d75541b](https://github.com/anatolykoptev/go-browser/commit/d75541b1520a13f898787ef4bc4397189ff60e14))


### Fixed

* add --no-sandbox to rod restart path and metrics test launcher ([bcfa5f9](https://github.com/anatolykoptev/go-browser/commit/bcfa5f9a2fde320b2a318c65b090afefbc39408c))
* atomic.Pointer for ContextPool.browser + reconnect TOCTOU fix ([34b85a1](https://github.com/anatolykoptev/go-browser/commit/34b85a18a1e00ee9d16a94a35bdf26a9c96861bf))
* centralize stealth application via pool onPageCreated hook ([b2c846f](https://github.com/anatolykoptev/go-browser/commit/b2c846fb591a0a2b9291f04319b0ea114ac53df5))
* CI race detection + lock ordering tests + preflight timeout ([ebd0c45](https://github.com/anatolykoptev/go-browser/commit/ebd0c458f254b9e5085a2d616e592a00e98ba6cb))
* DNS-rebind detection + EGRESS_ALLOW_LOCALHOST warning + TLS error surfacing ([85cca3c](https://github.com/anatolykoptev/go-browser/commit/85cca3c8d91fb901658ee93e065abacfd3d54212))
* gostall — use only lockorder+missingunlock+starvation analyzers ([842d0bc](https://github.com/anatolykoptev/go-browser/commit/842d0bc8a54f5c525759e5eb46a939ed7fc3e48d))
* gostall CI integration + ClosePage lock starvation fix ([bada74a](https://github.com/anatolykoptev/go-browser/commit/bada74a701082fcfe6abf91baeb7ed5e9de6bd26))
* HealthCheck method + Prometheus metrics endpoint ([e816b76](https://github.com/anatolykoptev/go-browser/commit/e816b76c85700609a5d02c092f8b9ce20e8772ba))
* humanize race, Reap stale pages, DNS-rebind fallback, dead code, action tests ([9e7961b](https://github.com/anatolykoptev/go-browser/commit/9e7961b131ac850a73798dee6c3692c9bfaaacb4))
* log silent session name downgrade + proxy auth skip ([faa52c3](https://github.com/anatolykoptev/go-browser/commit/faa52c37ab332635d5ca8110f92f9d9033ec48b5))
* misc low-priority bugs — retry backoff, jitter, error logging, validation ([4f7725c](https://github.com/anatolykoptev/go-browser/commit/4f7725c59becd3265488840f02f3d396341e8835))
* placeholder ready channel safety + page creation timeout ([8072b73](https://github.com/anatolykoptev/go-browser/commit/8072b7383067325d20238b6decaf22617a5f6ae2))
* reconnect page invalidation + generation counter + LostConnection ([16f302e](https://github.com/anatolykoptev/go-browser/commit/16f302e96ee7daeb83a54a3a302cf3dc5516e256))
* ring buffer eviction metrics + usage ratio watermark ([96942e0](https://github.com/anatolykoptev/go-browser/commit/96942e058d89512278f6238f41d8aefd9ceab630))
* **rod:** add --no-sandbox to rod backend launcher for CI compatibility ([6c0a02f](https://github.com/anatolykoptev/go-browser/commit/6c0a02fa2d06d56737e3deed9ae41637aa52bec1))
* stale context backoff + adoptExistingPage rediscovery + selftest cleanup ([f639f84](https://github.com/anatolykoptev/go-browser/commit/f639f84366d7f39ff222ea749dfbee1c9a684c72))
* staticcheck QF1012 — use fmt.Fprintf instead of WriteString(Sprintf) ([41f69b8](https://github.com/anatolykoptev/go-browser/commit/41f69b830d514c1e68f2a287c1fdbd8f71a6fbf7))
* structured CDP error type + stop error swallowing ([a99bb74](https://github.com/anatolykoptev/go-browser/commit/a99bb74a2101b4f99f5904478584db5f4a991a93))
* **test:** navigate to https:// for userAgentData — not available on about:blank ([17b243d](https://github.com/anatolykoptev/go-browser/commit/17b243de1e69a669a366e44e1dc2fd14dddafb63))
* **tests:** 3 integration test fixes for CI runner compatibility ([4da468a](https://github.com/anatolykoptev/go-browser/commit/4da468a0bbf60db13ad09fdf85b08122d738927f))

## [0.20.1](https://github.com/anatolykoptev/go-browser/compare/v0.20.0...v0.20.1) (2026-07-16)


### Fixed

* add Wait field to RenderRequest for wait strategy selection ([59f19ce](https://github.com/anatolykoptev/go-browser/commit/59f19ce4b496cb03b3630911b80929a0162824f9))
* add Wait field to RenderRequest for wait strategy selection ([f4cc30d](https://github.com/anatolykoptev/go-browser/commit/f4cc30d34954c2e9812543505b14ccc3d528b96e))
* bound networkidle wait to 80% of timeout for HTML extraction ([2379dba](https://github.com/anatolykoptev/go-browser/commit/2379dba224f38b9670349f7d6e4771bd376305a0))
* bound networkidle wait to 80% of timeout, keep 20% for HTML ([f607f56](https://github.com/anatolykoptev/go-browser/commit/f607f5665d720b61f4d290e1a77c8b7c6cfdb13b))
* **context-pool:** close TOCTOU in default-context discovery + positive-contract test ([e7e649e](https://github.com/anatolykoptev/go-browser/commit/e7e649e68d24eb4ae81f776332e214a19d594af2))
* **context-pool:** exclude pool-created incognito contexts from default discovery ([3a78bf9](https://github.com/anatolykoptev/go-browser/commit/3a78bf99286871f349b4f9cc7d75ab11a1ac6fea))
* **context-pool:** exclude pool-created incognito contexts from default discovery ([c14fe41](https://github.com/anatolykoptev/go-browser/commit/c14fe4101f1a0d247f88fca359a5e13e2e2aa80f))
* localhost SSRF bypass + non-fatal locale/timezone override conflict ([27c4fb0](https://github.com/anatolykoptev/go-browser/commit/27c4fb0b62bf7785d55406f0dd1b39fe2caf7b17))
