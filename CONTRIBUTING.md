# Contributing

## Getting Started
1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Run tests: `go test ./... -count=1 -race`
4. Open a PR

## Guidelines
- Follow existing patterns (pgx raw SQL, Gin handlers, slog logging)
- Write tests with testcontainers-go for DB integration
- Add migrations for schema changes

## Commit Style
Use conventional commits: `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`
