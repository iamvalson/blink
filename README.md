# Blink

Blink is a Go service for reliable, asynchronous publishing to social
platforms. The API persists publishing intent in PostgreSQL, places work in
Redis/Asynq, and a separate worker performs publishing with retries,
idempotency checks, rate-limit handling, OAuth refresh, and durable failure
tracking.

The repository also contains a Next.js web application in
[`web/blink-web`](web/blink-web).

## Architecture

```text
Next.js web app
       |
       v
Go API --> PostgreSQL
  |           |
  +--> Redis / Asynq <--- Go worker --> Twitter/X or YouTube
```

The API owns authentication, validation, persistence, idempotency, and
enqueueing. The worker owns platform calls, OAuth refresh, retry decisions,
rate-limit handling, and publication result persistence. PostgreSQL is the
authoritative store for application and publication state; Redis is used for
queueing and worker infrastructure.

## Current Features

- JWT-based user authentication
- Twitter/X and YouTube connector abstractions
- OAuth account linking with encrypted token storage
- PostgreSQL-backed posts, targets, attempts, outbox events, and failure history
- Redis-backed Asynq publishing jobs
- Bounded business retries with exponential backoff
- Rate-limit-aware retry scheduling
- Ambiguous-publication reconciliation
- Database-backed dead-letter failure records
- Structured Zerolog logging and Prometheus metrics
- Graceful API and worker shutdown on `SIGINT` and `SIGTERM`
- Mock Twitter/X publishing mode for local testing

## Repository Layout

```text
cmd/
  api/main.go                 API process
  worker/main.go              Asynq worker process
internal/
  api/                        Chi router and HTTP handlers
  auth/                       JWT, password, and token encryption
  config/                     Environment-backed configuration
  connectors/                 Platform connector interfaces and implementations
  jobs/                       Asynq client and job payloads
  metrics/                    Prometheus metrics
  outbox/                     Database outbox dispatcher
  storage/                    PostgreSQL repositories
  worker/                     Publish processor and retry logic
migrations/                   PostgreSQL schema migrations
docs/                         Reliability and operational notes
infra/docker-compose.yml      Local PostgreSQL and Redis
web/blink-web/                Next.js frontend
tests/                        Integration tests
```

## Prerequisites

- Go 1.26 or newer
- Docker and Docker Compose
- PostgreSQL client tools (`psql`) for migrations
- Node.js and pnpm for the frontend

## Configuration

The Go processes load `.env` when it is present. The Makefile also includes
and exports `.env`, so create that file before using `make` targets.

The main settings are:

| Variable                | Default                                            | Used by                                              |
| ----------------------- | -------------------------------------------------- | ---------------------------------------------------- |
| `DATABASE_URL`          | `postgres://blink:devpass@localhost:5432/blink_db` | API and worker                                       |
| `REDIS_URL`             | `redis://localhost:6379`                           | API and worker                                       |
| `PORT`                  | `8000`                                             | API                                                  |
| `ENV`                   | `development`                                      | API logging                                          |
| `LOG_LEVEL`             | `info`                                             | API logging                                          |
| `SHUTDOWN_TIMEOUT`      | `30s`                                              | API and worker                                       |
| `ENCRYPTION_KEY`        | empty                                              | API token encryption and worker publishing           |
| `PLATFORM_MODE`         | `real`                                             | Worker; set to `mock` for local Twitter/X publishing |
| `TWITTER_CLIENT_ID`     | empty                                              | API and worker                                       |
| `TWITTER_CLIENT_SECRET` | empty                                              | API and worker                                       |
| `TWITTER_CALLBACK_URL`  | empty                                              | API and worker                                       |
| `YOUTUBE_CLIENT_ID`     | empty                                              | API and worker                                       |
| `YOUTUBE_CLIENT_SECRET` | empty                                              | API and worker                                       |
| `YOUTUBE_CALLBACK_URL`  | empty                                              | API and worker                                       |

`SHUTDOWN_TIMEOUT` accepts Go duration syntax such as `5s` or `1m`. Invalid
and non-positive values fall back to 30 seconds. Never commit real secrets.

Example local `.env`:

```dotenv
DATABASE_URL=postgres://blink:devpass@localhost:5432/blink_db
REDIS_URL=redis://localhost:6379
ENCRYPTION_KEY=replace-with-a-local-key
PLATFORM_MODE=mock
SHUTDOWN_TIMEOUT=30s
```

## Local Setup

Start PostgreSQL and Redis:

```bash
make up
```

Apply the schema migrations in order. The repository currently contains five
numbered migration pairs:

```bash
for file in migrations/*.up.sql; do
  psql -v ON_ERROR_STOP=1 "${DATABASE_URL}?sslmode=disable" -f "$file"
done
```

`make db-setup` starts the infrastructure and applies the initial migration.
For a fresh checkout, use the loop above afterward to apply the remaining
migrations as well.

Run the API and worker in separate terminals:

```bash
make run-api
PLATFORM_MODE=mock make run-worker
```

The API listens on `http://localhost:8000` by default. Press Ctrl+C or send
`SIGTERM` to let active HTTP requests and worker jobs drain within the
configured shutdown timeout. See [Graceful Shutdown](docs/graceful_shutdown.md)
for the lifecycle and container guidance.

## API Routes

Public routes:

```text
GET  /health
GET  /metrics
POST /auth/signup
POST /auth/login
POST /auth/logout
```

OAuth routes are registered for each configured connector:

```text
GET /auth/twitter
GET /auth/twitter/callback
GET /auth/youtube
GET /auth/youtube/callback
```

Post routes require JWT authentication:

```text
POST /api/v1/posts
GET  /api/v1/posts/{id}
POST /api/posts
GET  /api/posts/{id}
```

The `/api/posts` paths are retained as compatibility aliases for the versioned
post routes.

## Publishing and Reliability

Publishing follows this shape:

```text
HTTP request
    |
    v
Persist post, targets, attempts, and outbox event
    |
    v
Outbox dispatcher -> Redis/Asynq
    |
    v
Worker loads authoritative state from PostgreSQL
    |
    +--> refresh OAuth token when needed
    +--> publish through a platform connector
    +--> persist success, retry, reconciliation, or terminal failure
```

Transient failures such as network errors, platform `5xx` responses, and
rate-limit responses are retryable. Platform-provided retry timing takes
precedence over exponential backoff. Ambiguous external results are recorded
as `UNKNOWN` and reconciled before another publish is attempted.

The worker uses explicit database-backed business retries and sets Asynq's
native retry count so the two retry systems do not multiply one another.
Terminal failures are recorded in the database-backed dead-letter tables.
See [`docs/retry.md`](docs/retry.md) and
[`docs/oauth_refresh.md`](docs/oauth_refresh.md) for the detailed behavior.

### Mock Publishing

Set `PLATFORM_MODE=mock` to exercise the publishing pipeline without outbound
Twitter/X requests:

```bash
PLATFORM_MODE=mock go run cmd/worker/main.go
```

The mock connector still exercises queueing, repository updates, publication
attempts, and post status transitions. See [`docs/mock_mode.md`](docs/mock_mode.md)
for verification queries.

## Graceful Shutdown

Both processes handle `SIGINT` and `SIGTERM` without calling `os.Exit` during
normal cleanup.

- The API stops accepting new HTTP work with `http.Server.Shutdown`, stops the
  outbox dispatcher, closes its Asynq client, and then closes PostgreSQL.
- The worker uses Asynq's official graceful shutdown with `SHUTDOWN_TIMEOUT`,
  waits for the worker to stop, then closes its retry client and PostgreSQL.
- If the deadline expires, unfinished jobs are not marked successful. Existing
  Asynq recovery and Blink retry/reconciliation semantics remain responsible
  for recovery.
- Startup failures clean up resources that were initialized before the failure.

## Frontend

The Next.js application lives in [`web/blink-web`](web/blink-web). Start it
with:

```bash
cd web/blink-web
pnpm install
pnpm dev
```

The default development URL is `http://localhost:3000`.

## Testing

Run all Go tests:

```bash
go test ./...
```

Run static analysis for the main processes and lifecycle packages:

```bash
go vet ./cmd/api ./cmd/worker ./internal/config ./internal/worker
```

The test suite includes API and service tests, connector tests, retry and
publishing processor tests, OAuth refresh coverage, metrics tests, and an
integration publishing scenario. External platform calls are mocked in tests.

## Useful Commands

```bash
make up             # Start PostgreSQL and Redis
make down           # Stop local infrastructure
make logs           # Follow infrastructure logs
make ps             # Show infrastructure status
make run-api        # Start the API
make run-worker     # Start the worker
make db-setup       # Start infrastructure and apply initial schema
make db-reset       # Delete local volumes and recreate the database
```

## Roadmap

The current focus is Phase 3 reliability engineering: idempotency, bounded
retries, dead-letter handling, OAuth refresh, rate-limit awareness,
observability, and graceful shutdown. Future work may include scheduling,
additional platforms, media processing, analytics, distributed rate limiting,
tracing, and operator tooling for replaying failures.

## License

Blink is under active development. See [`LICENSE`](LICENSE) for the current
license information.
