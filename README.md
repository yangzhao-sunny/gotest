# taskmgr

Multi-user task management HTTP/JSON API service.

## Prerequisites

- Go 1.25+
- Docker + Docker Compose
- `oapi-codegen` (for regenerating stubs): `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`
- `golang-migrate` CLI (optional, for manual migration): `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest`

## Quick Start (Docker Compose)

```bash
# Start Postgres + Redis, run migrations, start the service
docker-compose up --build

# Service is available at http://localhost:8080
curl http://localhost:8080/healthz
```

## Local Development

```bash
# 1. Copy and edit env vars
cp .env/local.env .env/.local.env
# Uncomment and set DATABASE_URL, REDIS_ADDR, JWT_SECRET

# 2. Start dependencies only
docker-compose up -d postgres redis

# 3. Run migrations
export TASKMGR_DB_DSN=postgres://app:secret@localhost:5432/appdb?sslmode=disable
export TASKMGR_JWT_SECRET=dev-secret
go run ./cmd/migrate up

# 4. Start the server
go run ./cmd/server/
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `TASKMGR_DB_DSN` | *(required)* | PostgreSQL DSN |
| `TASKMGR_JWT_SECRET` | *(required)* | HS256 signing secret |
| `TASKMGR_REDIS_ADDR` | `localhost:6379` | Redis address |
| `TASKMGR_PORT` | `8080` | HTTP listen port |
| `TASKMGR_OTEL_ENABLED` | `true` | Enable OpenTelemetry |
| `TASKMGR_OTEL_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint |

## Running Tests

```bash
# Unit tests (fast, no containers)
make test

# Integration tests (spins up Postgres + Redis via testcontainers)
make test-integration
```

## Database Migrations

```bash
# Apply all pending migrations
go run ./cmd/migrate up

# Roll back last migration
go run ./cmd/migrate down

# Or via built binary
make build
./dist/taskmgr-migrate up
```

## Regenerate API Client Stubs

```bash
make generate
# Produces api/gen/server.gen.go from api/openapi.yaml
```

## Generate Example Client (curl)

```bash
# Register
curl -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"s3cret123","display_name":"Alice"}'

# Login
TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"s3cret123"}' | jq -r .access_token)

# Create project
curl -X POST http://localhost:8080/v1/projects \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"My Project"}'
```

## Build

```bash
make build
# Produces dist/taskmgr and dist/taskmgr-migrate
```

## Publish OpenAPI Contract

```bash
make publish
# Copies api/openapi.yaml → contracts/openapi.yaml
```
