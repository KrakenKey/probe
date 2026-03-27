# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in KrakenKey Probe, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@krakenkey.io** with:

- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We will acknowledge your report within **48 hours** and aim to provide a fix or mitigation within **7 days** for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |

## Security Considerations

### What the Probe Collects

The probe connects to TLS endpoints and extracts **publicly available** certificate metadata (subject, issuer, SANs, expiry, etc.) and connection metadata (TLS version, cipher suite, handshake latency). It does not intercept, decrypt, or inspect application-layer traffic.

### API Key Handling

- API keys are passed via environment variables or config files and are sent as Bearer tokens over HTTPS.
- Keys are never logged, even at debug level.
- Use Kubernetes Secrets or equivalent mechanisms to manage API keys in production.

### Container Security

- The official Docker image runs as a non-root user (UID 65532).
- The image is based on `distroless/static-debian12` to minimize attack surface.
- No shell or package manager is included in the production image.

### Network

- Outbound connections are made to configured TLS endpoints and the KrakenKey API (`https://api.krakenkey.io` by default).
- The health server listens on a configurable port (default `8080`) and serves read-only status information.
- No inbound connections are required beyond the health endpoint.
