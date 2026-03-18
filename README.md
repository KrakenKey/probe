# KrakenKey Probe

[![CI](https://github.com/krakenkey/probe/actions/workflows/ci.yaml/badge.svg)](https://github.com/krakenkey/probe/actions/workflows/ci.yaml)
[![Release](https://github.com/krakenkey/probe/releases/latest)](https://github.com/krakenkey/probe/releases/latest)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

A lightweight TLS monitoring agent that scans endpoints for certificate health and reports results to the [KrakenKey](https://krakenkey.io) platform. Run it on your own infrastructure to monitor internal and external TLS certificates from a single dashboard.

The probe connects to configured endpoints via TLS, extracts certificate and connection metadata (expiry, issuer, SANs, chain validity, TLS version, handshake latency), and sends results to the KrakenKey API on a configurable schedule.

## Quick Start

```bash
docker run -d \
  -e KK_PROBE_API_KEY="kk_your_api_key" \
  -e KK_PROBE_ENDPOINTS="example.com:443,api.example.com:443" \
  -e KK_PROBE_NAME="my-probe" \
  -v probe_state:/var/lib/krakenkey-probe \
  ghcr.io/krakenkey/probe:latest
```

## Configuration

The probe is configured via a YAML file and/or environment variables. Environment variables take precedence over YAML values.

### YAML Config

```yaml
api:
  url: "https://api.krakenkey.io"       # KK_PROBE_API_URL
  key: "kk_..."                          # KK_PROBE_API_KEY (required)

probe:
  id: ""                                 # KK_PROBE_ID (auto-generated if empty)
  name: "my-probe"                       # KK_PROBE_NAME
  mode: "self-hosted"                    # KK_PROBE_MODE (self-hosted | hosted)
  region: ""                             # KK_PROBE_REGION
  interval: "60m"                        # KK_PROBE_INTERVAL (min: 1m, max: 24h)
  timeout: "10s"                         # KK_PROBE_TIMEOUT
  state_file: "/var/lib/krakenkey-probe/state.json"  # KK_PROBE_STATE_FILE

endpoints:                               # KK_PROBE_ENDPOINTS (comma-separated host:port)
  - host: "example.com"
    port: 443
    sni: ""                              # optional SNI override
  - host: "internal.corp.net"
    port: 8443

health:
  enabled: true                          # KK_PROBE_HEALTH_ENABLED
  port: 8080                             # KK_PROBE_HEALTH_PORT

logging:
  level: "info"                          # KK_PROBE_LOG_LEVEL (debug|info|warn|error)
  format: "json"                         # KK_PROBE_LOG_FORMAT (json|text)
```

### Environment Variable Reference

| Variable | Default | Description |
|---|---|---|
| `KK_PROBE_API_URL` | `https://api.krakenkey.io` | KrakenKey API base URL |
| `KK_PROBE_API_KEY` | (required) | API key, must start with `kk_` |
| `KK_PROBE_ID` | (auto-generated) | Probe ID, persisted to state file |
| `KK_PROBE_NAME` | | Human-friendly probe name |
| `KK_PROBE_MODE` | `self-hosted` | `self-hosted` or `hosted` |
| `KK_PROBE_REGION` | | Geographic region label |
| `KK_PROBE_INTERVAL` | `60m` | Scan interval (Go duration) |
| `KK_PROBE_TIMEOUT` | `10s` | Per-endpoint TLS dial timeout |
| `KK_PROBE_STATE_FILE` | `/var/lib/krakenkey-probe/state.json` | State file path |
| `KK_PROBE_ENDPOINTS` | | Comma-separated `host:port` pairs |
| `KK_PROBE_HEALTH_ENABLED` | `true` | Enable health endpoint |
| `KK_PROBE_HEALTH_PORT` | `8080` | Health endpoint port |
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
```

1. On startup, the probe loads its config, generates or reads its probe ID, and registers with the KrakenKey API.
2. It immediately runs the first scan cycle: connects to each endpoint via TLS, extracts certificate metadata, and sends results to the API.
3. After the first scan, the `/readyz` endpoint returns `200 OK`.
4. Subsequent scans run on the configured interval.
5. On `SIGINT` or `SIGTERM`, the probe finishes the current scan cycle and shuts down.

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

```json
{
  "status": "ok",
  "version": "0.1.0",
  "probeId": "a1b2c3d4-...",
  "mode": "self-hosted",
  "region": "us-east-1",
  "lastScan": "2026-03-17T12:00:00Z",
  "nextScan": "2026-03-17T13:00:00Z"
}
```

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
