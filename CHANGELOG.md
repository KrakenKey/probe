# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- State file now written with `0600` permissions instead of `0644` (#31)
- Graceful-shutdown timeout uses a `time.Second` literal instead of a raw nanosecond constant; Go bumped to 1.24.0 (#31)

## [0.2.0] - 2026-05-03

### Added

- `POST /scan` endpoint — authenticated on-demand scan API, disabled by default; enable with `KK_PROBE_SCAN_API_ENABLED=true` (or `scan_api.enabled` in YAML)
- Bearer-token authentication for the scan API via `KK_PROBE_SCAN_API_SECRET` (`scan_api.secret`); startup fails if the secret is shorter than 32 characters while the scan API is enabled
- `ScanAPIConfig` struct in probe configuration
- Used by the KrakenKey API's `POST /public/scan` (`PublicScanModule`) to proxy free TLS scan requests to hosted probes
- **On-Demand Scan API** section in README covering config, request format, response shape, and security guidance (#16)

## [0.1.0] - 2026-03-17

### Added

- TLS endpoint scanner with certificate metadata extraction (subject, SANs, issuer, chain, expiry, fingerprint, key type, signature algorithm)
- Connection metadata collection (TLS version, cipher suite, handshake latency, OCSP stapling)
- Three operating modes: `standalone`, `connected`, and `hosted`
- KrakenKey API integration for probe registration and result reporting
- Health check server with `/healthz` and `/readyz` endpoints
- Configurable scan interval (1m to 24h) with immediate first scan on startup
- YAML config file with environment variable overrides (`KK_PROBE_*` prefix)
- Persistent probe ID across restarts via state file
- Graceful shutdown on SIGINT/SIGTERM
- Multi-platform Docker images (linux/amd64, linux/arm64) via GoReleaser
- CI pipeline with lint, test, and build matrix
- Kubernetes deployment example with ConfigMap, Secrets, and health probes

[0.1.0]: https://github.com/krakenkey/probe/releases/tag/v0.1.0
[0.2.0]: https://github.com/krakenkey/probe/releases/tag/v0.2.0
