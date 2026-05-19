# Changelog

Notable changes to the KrakenKey Probe. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/). Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [v0.2.0] — 2026-05-03

### Added
- `POST /scan` — authenticated on-demand scan endpoint. Enabled via `KK_PROBE_SCAN_API_ENABLED=true`; protected by a Bearer token (`KK_PROBE_SCAN_API_SECRET`, minimum 32 characters enforced at startup).
- `ScanAPIConfig` struct in the configuration (`scan_api.enabled` / `scan_api.secret` YAML keys).
- Startup validation: fails with a clear error message if the scan API secret is shorter than 32 characters.
- Used by KrakenKey API's `POST /public/scan` to proxy free TLS scan requests to the hosted probe.
- `KK_PROBE_SCAN_API_ENABLED` and `KK_PROBE_SCAN_API_SECRET` env vars documented in README.
- **On-Demand Scan API** section added to README covering config, request format, response shape, and security guidance.
- **Troubleshooting** entry added for secret-length startup error.

---

## [v0.1.0] — 2026-03-27

### Added
- Initial release: TLS endpoint scanning with certificate chain validation, expiry checking, and connection health metrics (TLS version, cipher suite, OCSP stapling, latency).
- Three operating modes: `standalone` (local YAML config, no API), `connected` (user API key, reports to platform), `hosted` (service key, receives config from API by region).
- Dynamic endpoint reloading in `connected` and `hosted` modes — polls API every 60 seconds; triggers immediate scan if endpoint list changes.
- `/healthz` and `/readyz` HTTP health endpoints.
- `--healthcheck` CLI flag for Docker-native health checks (`probe --healthcheck`).
- Persistent JSON state file for probe ID and scan history.
- Multi-architecture Docker image (`ghcr.io/krakenkey/probe`) for amd64 and arm64.
- `CHANGELOG.md`, `CONTRIBUTING.md`, and `SECURITY.md` documentation files.
- Pre-built goreleaser release pipeline with SHA-256 checksums.
