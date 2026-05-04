# KrakenKey Probe

[![CI](https://github.com/krakenkey/probe/actions/workflows/ci.yaml/badge.svg)](https://github.com/krakenkey/probe/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/KrakenKey/probe)](https://github.com/krakenkey/probe/releases/latest)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

A lightweight TLS monitoring agent that scans endpoints for certificate health and reports results to the [KrakenKey](https://krakenkey.io) platform. Run it on your own infrastructure to monitor internal and external TLS certificates from a single dashboard.

The probe connects to configured endpoints via TLS, extracts certificate and connection metadata (expiry, issuer, SANs, chain validity, TLS version, handshake latency), and sends results to the KrakenKey API on a configurable schedule.

## Quick Start

### Standalone (no API key needed)

```bash
docker run -d \
  -e KK_PROBE_ENDPOINTS="example.com:443,api.example.com:443" \
  -e KK_PROBE_NAME="my-probe" \
  -v probe_state:/var/lib/krakenkey-probe \
  ghcr.io/krakenkey/probe:latest
```

### Connected (reports to KrakenKey)

```bash
docker run -d \
  -e KK_PROBE_MODE="connected" \
  -e KK_PROBE_API_KEY="kk_your_api_key" \
  -e KK_PROBE_NAME="my-probe" \
  -v probe_state:/var/lib/krakenkey-probe \
  ghcr.io/krakenkey/probe:latest
```

## Operating Modes

The probe supports three operating modes:

| Mode | API Key | Endpoints | Description |
|---|---|---|---|
| `standalone` | Not required | Defined locally in config/env | Fully local. Scans endpoints and logs results to console. No API communication. |
| `connected` | Required (`kk_` prefix) | Managed via KrakenKey dashboard | Endpoints fetched from API. Results reported to KrakenKey for dashboard monitoring. |
| `hosted` | Required (service key) | Managed by KrakenKey | Fully managed by KrakenKey infrastructure. Probe ID, name, and region are pre-configured. |

Set the mode with `KK_PROBE_MODE` or `probe.mode` in the YAML config. Default: `standalone`.

## CLI Flags

```
krakenkey-probe [flags]

Flags:
  --config <path>    Path to probe.yaml config file
  --version          Print version and exit
  --healthcheck      Check health endpoint (http://localhost:8080/healthz) and exit with 0 (healthy) or 1 (unhealthy)
```

## Configuration

The probe is configured via a YAML file and/or environment variables. Configuration is loaded in order of precedence (highest wins):

1. **Environment variables** (`KK_PROBE_*` prefix)
2. **YAML config file** (if `--config` is provided)
3. **Built-in defaults**

### YAML Config

```yaml
api:
  url: "https://api.krakenkey.io"        # KK_PROBE_API_URL
  key: ""                                # KK_PROBE_API_KEY (required for connected/hosted modes)

probe:
  id: ""                                 # KK_PROBE_ID (auto-generated if empty)
  name: "my-probe"                       # KK_PROBE_NAME
  mode: "standalone"                     # KK_PROBE_MODE (standalone | connected | hosted)
  region: ""                             # KK_PROBE_REGION (required for hosted mode)
  interval: "60m"                        # KK_PROBE_INTERVAL (min: 1m, max: 24h)
  timeout: "10s"                         # KK_PROBE_TIMEOUT
  state_file: "/var/lib/krakenkey-probe/state.json"  # KK_PROBE_STATE_FILE

endpoints:                               # KK_PROBE_ENDPOINTS (comma-separated host:port)
  - host: "example.com"
    port: 443
    sni: ""                              # optional: override the SNI hostname sent during TLS handshake
  - host: "internal.corp.net"
    port: 8443

health:
  enabled: true                          # KK_PROBE_HEALTH_ENABLED
  port: 8080                             # KK_PROBE_HEALTH_PORT

scan_api:
  enabled: false                         # KK_PROBE_SCAN_API_ENABLED
  secret: ""                             # KK_PROBE_SCAN_API_SECRET (min 32 chars, required if enabled)

logging:
  level: "info"                          # KK_PROBE_LOG_LEVEL (debug|info|warn|error)
  format: "json"                         # KK_PROBE_LOG_FORMAT (json|text)
```

### SNI Override

Use the `sni` field when the hostname you connect to differs from the hostname expected by the TLS certificate. This is common with load balancers, CDNs, or internal services behind a reverse proxy. If omitted, the `host` value is used as the SNI hostname.

### Environment Variable Reference

| Variable | Default | Description |
|---|---|---|
| `KK_PROBE_API_URL` | `https://api.krakenkey.io` | KrakenKey API base URL |
| `KK_PROBE_API_KEY` | | API key (`kk_` prefix). Required for `connected` and `hosted` modes. |
| `KK_PROBE_ID` | (auto-generated) | Probe ID, persisted to state file |
| `KK_PROBE_NAME` | | Human-friendly probe name |
| `KK_PROBE_MODE` | `standalone` | `standalone`, `connected`, or `hosted` |
| `KK_PROBE_REGION` | | Geographic region label (required for `hosted` mode) |
| `KK_PROBE_INTERVAL` | `60m` | Scan interval (Go duration, e.g. `30m`, `1h`) |
| `KK_PROBE_TIMEOUT` | `10s` | Per-endpoint TLS dial timeout |
| `KK_PROBE_STATE_FILE` | `/var/lib/krakenkey-probe/state.json` | State file path |
| `KK_PROBE_ENDPOINTS` | | Comma-separated `host:port` pairs. Port defaults to `443` if omitted. |
| `KK_PROBE_HEALTH_ENABLED` | `true` | Enable health endpoint |
| `KK_PROBE_HEALTH_PORT` | `8080` | Health endpoint port |
| `KK_PROBE_SCAN_API_ENABLED` | `false` | Enable the authenticated on-demand scan API (`POST /scan`) |
| `KK_PROBE_SCAN_API_SECRET` | | Bearer token secret for `POST /scan` authentication (min 32 chars). Required when scan API is enabled. |
| `KK_PROBE_LOG_LEVEL` | `info` | Log level |
| `KK_PROBE_LOG_FORMAT` | `json` | Log format |

## Docker Compose

```yaml
services:
  krakenkey-probe:
    image: ghcr.io/krakenkey/probe:latest
    container_name: krakenkey-probe
    restart: unless-stopped
    environment:
      KK_PROBE_MODE: "connected"
      KK_PROBE_API_KEY: "kk_your_api_key_here"
      KK_PROBE_NAME: "my-probe"
      KK_PROBE_ENDPOINTS: "example.com:443,api.example.com:443"
      KK_PROBE_INTERVAL: "30m"
    volumes:
      - probe_state:/var/lib/krakenkey-probe
    ports:
      - "8080:8080"

volumes:
  probe_state:
```

## Kubernetes

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: krakenkey-probe
data:
  probe.yaml: |
    api:
      url: "https://api.krakenkey.io"
    probe:
      name: "k8s-probe"
      interval: "30m"
    endpoints:
      - host: "example.com"
        port: 443
      - host: "api.example.com"
        port: 443
    health:
      enabled: true
      port: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: krakenkey-probe
spec:
  replicas: 1
  selector:
    matchLabels:
      app: krakenkey-probe
  template:
    metadata:
      labels:
        app: krakenkey-probe
    spec:
      containers:
        - name: probe
          image: ghcr.io/krakenkey/probe:latest
          args: ["--config", "/etc/krakenkey-probe/probe.yaml"]
          env:
            - name: KK_PROBE_API_KEY
              valueFrom:
                secretKeyRef:
                  name: krakenkey-probe-secret
                  key: api-key
          ports:
            - containerPort: 8080
              name: health
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /readyz
              port: health
            initialDelaySeconds: 5
            periodSeconds: 10
          volumeMounts:
            - name: config
              mountPath: /etc/krakenkey-probe
              readOnly: true
            - name: state
              mountPath: /var/lib/krakenkey-probe
      volumes:
        - name: config
          configMap:
            name: krakenkey-probe
        - name: state
          emptyDir: {}
```

## Building from Source

```bash
# Clone
git clone https://github.com/krakenkey/probe.git
cd probe

# Build
go build -ldflags="-s -w -X main.version=0.1.0" -o krakenkey-probe ./cmd/probe

# Run
./krakenkey-probe --config probe.example.yaml
```

### Cross-compile

```bash
# Linux ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o krakenkey-probe ./cmd/probe

# macOS ARM64 (Apple Silicon)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o krakenkey-probe ./cmd/probe
```

## How It Works

```
                         +-----------+
                         | KrakenKey |
                         |    API    |
                         +-----^-----+
                               |
                          POST /probes/report
                               |
+----------------+       +-----+-----+       +------------------+
|  probe.yaml /  | ----> | KrakenKey | ----> | TLS Endpoints    |
|  env vars      |       |   Probe   |       | (host:port)      |
+----------------+       +-----------+       +------------------+
                               |
                         GET /healthz
                         GET /readyz
                         POST /scan  (when scan API enabled)
```

1. On startup, the probe loads its config and generates or reads its probe ID. In `connected`/`hosted` modes, it registers with the KrakenKey API.
2. It immediately runs the first scan cycle: connects to each endpoint via TLS and extracts certificate metadata. In `connected`/`hosted` modes, results are sent to the API. In `standalone` mode, results are logged to the console.
3. After the first scan, the `/readyz` endpoint returns `200 OK`.
4. Subsequent scans run on the configured interval.
5. On `SIGINT` or `SIGTERM`, the probe finishes the current scan cycle and shuts down gracefully.

### What Gets Collected

For each endpoint, the probe extracts:

**Connection metadata:**
- TLS protocol version (1.0, 1.1, 1.2, 1.3)
- Negotiated cipher suite
- TLS handshake latency (ms)
- OCSP stapling status

**Certificate metadata:**
- Subject CN and SANs
- Issuer chain
- Serial number
- Validity period and days until expiry
- Key type and size (RSA/ECDSA/Ed25519)
- Signature algorithm
- SHA-256 fingerprint
- Chain depth and completeness
- Trust status (verified against system cert pool)

## API Key Setup

1. Log in to [KrakenKey](https://app.krakenkey.io)
2. Navigate to **API Keys** in the dashboard
3. Create a new API key
4. Set `KK_PROBE_API_KEY` to the generated key (starts with `kk_`)

## Health Endpoints

| Endpoint | Description |
|---|---|
| `GET /healthz` | Always returns `200 OK` with probe status, version, mode, and scan times |
| `GET /readyz` | Returns `503` until the first scan completes, then `200 OK` |

### `/healthz` Response

Always returns `200 OK`:

```json
{
  "status": "ok",
  "version": "0.1.0",
  "probeId": "a1b2c3d4-...",
  "mode": "standalone",
  "region": "us-east-1",
  "lastScan": "2026-03-17T12:00:00Z",
  "nextScan": "2026-03-17T13:00:00Z"
}
```

### `/readyz` Response

Returns `503 Service Unavailable` before the first scan completes:

```json
{ "status": "not ready" }
```

Returns `200 OK` after the first scan completes:

```json
{ "status": "ready" }
```

## On-Demand Scan API

When `KK_PROBE_SCAN_API_ENABLED=true`, the probe exposes an authenticated `POST /scan` endpoint that triggers an immediate TLS scan of a given host.

This endpoint is used by the KrakenKey API to serve the public free TLS scanner at [krakenkey.io/scanner](https://krakenkey.io/scanner). The API proxies the request to a hosted probe using a shared secret.

### Authentication

Requests must include an `Authorization: Bearer <secret>` header where `<secret>` matches `KK_PROBE_SCAN_API_SECRET`.

### Request

```http
POST /scan
Authorization: Bearer <secret>
Content-Type: application/json

{
  "host": "example.com",
  "port": 443
}
```

### Response

Returns the same scan result structure as a scheduled scan cycle.

### Security

- The secret must be at least 32 characters. The probe refuses to start if `scan_api.enabled` is `true` and `scan_api.secret` is too short.
- The probe should not be exposed on a public network port when the scan API is enabled. Use a private network (e.g., Docker `internal-bridge`) and let the KrakenKey API proxy requests.
- SSRF protection (blocking private IP ranges) is enforced by the KrakenKey API layer before requests reach the probe.

## Troubleshooting

**"api.key is required"**
Set `KK_PROBE_API_KEY` or add `api.key` to your config file.

**"API authentication failed (HTTP 401): check your API key"**
The API key is invalid or expired. Generate a new one from the KrakenKey dashboard.

**"dial tcp: ... connection refused"**
The endpoint is not reachable from the probe's network. Check firewall rules, DNS resolution, and that the service is running on the expected port.

**"dial tcp: ... i/o timeout"**
The endpoint is not responding within the configured timeout. Increase `KK_PROBE_TIMEOUT` or check network connectivity.

**"API rate limited (HTTP 429)"**
The probe is sending reports too frequently. Increase `KK_PROBE_INTERVAL`.

**Probe ID keeps changing**
Ensure the state file path is persistent across restarts. When using Docker, mount a volume to `/var/lib/krakenkey-probe`.

## License

[AGPL-3.0](LICENSE)
