# Contributing to KrakenKey Probe

Thank you for your interest in contributing! This guide will help you get started.

## Getting Started

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [golangci-lint](https://golangci-lint.run/welcome/install-locally/) (for linting)
- Docker (optional, for container builds)

### Clone and Build

```bash
git clone https://github.com/krakenkey/probe.git
cd probe
go build -o krakenkey-probe ./cmd/probe
```

### Run Tests

```bash
go test ./... -race
```

### Run Linter

```bash
golangci-lint run
```

## Project Structure

```
cmd/probe/         Entry point (CLI flags, startup, shutdown)
internal/
  config/          Configuration loading, env overrides, validation
  health/          HTTP health check server (/healthz, /readyz)
  reporter/        KrakenKey API client (registration, reporting)
  scanner/         TLS endpoint scanner (certificate extraction)
  scheduler/       Scan scheduling and orchestration
  state/           Probe ID generation and persistence
```

## Development Workflow

1. **Fork** the repository and create a feature branch from `main`.
2. **Make your changes** with clear, focused commits.
3. **Run tests and lint** before pushing:
   ```bash
   go test ./... -race && golangci-lint run
   ```
4. **Open a pull request** against `main` with a clear description of what and why.

## Code Guidelines

- Keep dependencies minimal. The project intentionally uses only `gopkg.in/yaml.v3` beyond the standard library.
- Use `log/slog` for structured logging. Follow the existing patterns for log levels and context.
- Write tests for new functionality. Use `go test -race` to catch concurrency issues.
- Follow standard Go conventions (`gofmt`, `go vet`).

## Reporting Issues

- Use [GitHub Issues](https://github.com/krakenkey/probe/issues) to report bugs or request features.
- Include your Go version, OS/architecture, and probe version (`krakenkey-probe --version`).
- For bugs, include relevant log output (with `KK_PROBE_LOG_LEVEL=debug` if possible).

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0](LICENSE) license.
