# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-05-03

### Added

- `POST /scan` on-demand scan API: authenticated endpoint for triggering an immediate TLS scan of any reachable host. Returns the full scan result in the same format as scheduled scans.
- `KK_PROBE_SCAN_API_ENABLED` env var (default: `false`) — enables the `/scan` endpoint. Also configurable via YAML `scan_api.enabled`.
- `KK_PROBE_SCAN_API_SECRET` env var — Bearer token for `/scan` authentication (sent via `X-Probe-Secret` header); minimum 32 characters enforced at startup. Also configurable via YAML `scan_api.secret`.
- `ScanAPIConfig` struct added to the config package (`enabled`, `secret` fields).

[0.2.0]: https://github.com/krakenkey/probe/releases/tag/v0.2.0

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
