# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-05-03

### Added

- `POST /scan` on-demand TLS scan endpoint — enables external systems to request a scan on demand. Disabled by default; enable via `KK_PROBE_SCAN_API_ENABLED=true`.
- `KK_PROBE_SCAN_API_ENABLED` environment variable (default: `false`) — toggles the `/scan` endpoint.
- `KK_PROBE_SCAN_API_SECRET` environment variable — shared secret authenticating requests to `/scan` via the `X-Probe-Secret` header. Minimum 32 characters; validated at startup.
- `ScanAPIConfig` struct added to probe configuration for scan API settings.

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

[0.2.0]: https://github.com/krakenkey/probe/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/krakenkey/probe/releases/tag/v0.1.0
