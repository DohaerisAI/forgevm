# Contributing to ForgeVM

Thanks for your interest in contributing! ForgeVM is intentionally simple — no code generation, no frameworks, no magic.

## Getting Started

```bash
git clone https://github.com/DohaerisAI/forgevm
cd forgevm
make test        # run all tests
make build       # build binaries
```

## Development

- **Go 1.25+** required
- **No CGO** — pure Go, cross-compiles cleanly
- **SQLite** via `modernc.org/sqlite` (pure Go, no C compiler needed)

## Pull Requests

1. Fork the repo and create your branch from `main`
2. Write tests for any new functionality
3. Make sure `make test` passes
4. Keep PRs focused — one feature or fix per PR

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions small and focused
- Error messages should be lowercase, no trailing punctuation
- No unnecessary abstractions — three similar lines > premature helper

## Architecture

```
cmd/forgevm/          CLI + server entrypoint
cmd/forgevm-agent/    Guest agent (runs inside microVMs)
internal/             All server-side logic
sdk/python/           Python SDK
sdk/js/               TypeScript SDK
sdk/kotlin/           Kotlin SDK
web/                  React frontend
tui/                  Terminal dashboard
```

The orchestrator (`internal/orchestrator/`) manages sandbox lifecycle. Providers (`internal/providers/`) implement the actual VM management. The store (`internal/store/`) handles SQLite persistence.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
