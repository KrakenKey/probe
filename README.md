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

## Install from .deb/.rpm

Each release includes `.deb` and `.rpm` packages for Linux amd64 and arm64. They install the binary to `/usr/bin/krakenkey-probe`, a systemd unit (`krakenkey-probe.service`) that runs as a dedicated `krakenkey-probe` system user, and a default config at `/etc/krakenkey/probe.yaml`.

1. Download the package for your architecture from the [latest release](https://github.com/KrakenKey/probe/releases/latest). `checksums.txt` covers the packages too.
2. Install it:

   ```bash
   # Debian, Ubuntu
   sudo apt install ./krakenkey-probe_*_linux_amd64.deb

   # RHEL, Fedora, Amazon Linux
   sudo dnf install ./krakenkey-probe_*_linux_amd64.rpm
   ```

3. Edit `/etc/krakenkey/probe.yaml`. It starts in `standalone` mode with `example.com` as the only endpoint. For connected mode, set `probe.mode: "connected"` and `api.key`. The file is mode `0640`, owned by `root:krakenkey-probe`, and upgrades keep your edits.
4. Start the service. The package does not enable or start it for you:

   ```bash
   sudo systemctl enable --now krakenkey-probe
   journalctl -u krakenkey-probe -f
   ```

The probe ID is saved in `/var/lib/krakenkey-probe/state.json`. Removing the package stops and disables the service, and keeps the config, the state directory and the system user.

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
  secret: ""                             # KK_PROBE_SCAN_API_SECRET (min 32 chars when enabled)

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
| `KK_PROBE_SCAN_API_ENABLED` | `false` | Enable the `POST /scan` on-demand scan endpoint |
| `KK_PROBE_SCAN_API_SECRET` | | Bearer secret for `POST /scan` authentication (min 32 chars) |
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
+----------------+       +-----+-----+       +------------------+
                               |
                         GET /healthz
                         GET /readyz
                         POST /scan  (if scan_api.enabled)
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
- OCSP stapling status (whether a stapled response was present; see below)

**Certificate metadata:**
- Subject and SANs
- Issuer (the leaf certificate's issuer DN)
- Serial number
- Validity period and days until expiry
- Key type and size (RSA/ECDSA/Ed25519)
- Signature algorithm
- SHA-256 fingerprint
- Chain depth (number of certificates the server sent) and completeness (see below)
- Trust status (path validation against the system trust store; see below)

### Chain completeness, trust and AIA

The probe reports two separate chain fields, and they answer different questions:

- `chainComplete` is `true` only when the last certificate the server sends is a self-signed CA certificate, meaning the server included the root. Most correctly configured servers send the leaf plus intermediates and leave the root out, which TLS allows because clients already hold the root. For those servers `chainComplete` is `false`, and that is not a fault on its own. A server that sends only its leaf is also `false`.
- `trusted` is `true` when a path can be built from the leaf, through the intermediates the server sent, to a root in the system trust store, with every certificate in its validity period. This is the field that tells you whether clients can validate the chain. It does not check that the certificate matches the endpoint's host or SNI.

On Linux, which covers the Docker image and the Linux release binaries, trust is checked by Go's own `crypto/x509` verifier. It does **not** fetch a missing intermediate from the leaf's `authorityInformationAccess` (AIA) `caIssuers` URL, so an endpoint that serves only its leaf is reported as untrusted. That matches what clients without AIA fetching see, such as OpenSSL-based tools like `curl`, Go programs on Linux, and Java with default settings. Windows CryptoAPI/Schannel, macOS Security.framework and Chrome do fetch AIA, and Firefox usually accepts such a server too because it ships known intermediates in advance. So a site that loads in a desktop browser can still be reported as untrusted here. The fix is to configure the server to send its intermediate certificates.

On macOS, Go hands verification to the system verifier (Security.framework), which can fetch AIA intermediates. The macOS binaries may therefore report a leaf-only endpoint as trusted where the Linux build does not.

### OCSP stapling and revocation

`ocspStapled` is `true` when the server included a stapled OCSP response in the handshake. The probe does not parse or validate that response, so it reports nothing about revocation status. The field is omitted from JSON output when no response was stapled.

No stapled response is normal for many certificates today. Let's Encrypt, for example, removed OCSP URLs from its certificates in May 2025 and shut down its OCSP responders in August 2025, so servers using Let's Encrypt certificates have nothing to staple.

If revocation checking is added, do not assume an OCSP responder URL can be read from the leaf. CA/Browser Forum ballot SC104 (passed 2026-09-03) changes the `authorityInformationAccess` extension in subscriber certificates from MUST to SHOULD, so a compliant leaf may have no AIA extension at all, and the `id-ad-ocsp` access method inside it was already optional. Baseline Requirements §7.1.2.11.2 requires `crlDistributionPoints` in subscriber certificates that are not short-lived and have no `id-ad-ocsp` URL, so revocation checking needs a CRL path, not only OCSP. Short-lived subscriber certificates may carry neither, since CAs are not required to support revocation for them.

## On-Demand Scan API

The probe can expose an authenticated `POST /scan` endpoint for on-demand TLS scans. This is used by KrakenKey's hosted infrastructure to power the free public TLS scanner at `krakenkey.io/scanner`.

### Configuration

```yaml
scan_api:
  enabled: true
  secret: "<minimum-32-character-secret-here>"
```

Or via environment variables:

```bash
KK_PROBE_SCAN_API_ENABLED=true
KK_PROBE_SCAN_API_SECRET=your-secret-here  # min 32 chars
```

The probe refuses to start if `scan_api.enabled` is `true` and `secret` is shorter than 32 characters.

### Request

```
POST /scan
Authorization: Bearer <secret>
Content-Type: application/json

{"host": "example.com", "port": 443}
```

### Response

Returns the same TLS scan result structure as a scheduled scan: TLS version, cipher suite, certificate metadata, chain validity, handshake latency.

### Security

- Disabled by default (`KK_PROBE_SCAN_API_ENABLED=false`)
- SSRF protection is applied at the KrakenKey API layer before requests reach the probe
- Keep the probe off public network interfaces — expose it only on an internal bridge network
- Use a minimum 32-character cryptographically random secret

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

**"scan_api.secret must be at least 32 characters"**
Generate a longer secret: `openssl rand -hex 32`

## License

[AGPL-3.0](LICENSE)
