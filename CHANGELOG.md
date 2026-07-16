# Changelog

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
