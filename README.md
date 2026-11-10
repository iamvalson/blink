# Blink

Blink is a high-performance Go and Next.js service for reliable, scheduled, asynchronous publishing to social platforms. The API persists publishing intent in PostgreSQL using the transactional outbox pattern, enqueues background work into Redis/Asynq, and dedicated workers execute publishing pipelines with automatic retries, rate-limit awareness, OAuth token refresh, ambiguous failure reconciliation, and durable dead-letter tracking.

Blink also features real-time Server-Sent Events (SSE) status streaming, direct-to-storage media uploads (Cloudflare R2 and local filesystem), scheduled publishing with background lease claiming, and a modern Next.js web application in [`web`](web).

## Architecture

```text
                 +---------------------------------------------------+
                 |               Next.js Web Application             |
                 |              (Post Composer / Calendar)           |
                 +--------+--------------------------+---------------+
                          |                          |
            Direct Upload | (Presigned URL)          | HTTP / SSE Stream
                          v                          v
                 +-----------------+        +-----------------+
                 |  Cloudflare R2  |        |     Go API      |
                 | / Local Storage |        +--------+--------+
                 +--------+--------+                 |
                          ^                          | Persist Post / Outbox
                          |                          v
                          |                 +-----------------+
                          |                 |   PostgreSQL    |
                          |                 +--------+--------+
                          |                          |
                          |                          | Outbox Dispatcher
                          |                          v
                          |                 +-----------------+
                          |                 |  Redis / Asynq  |
                          |                 +--------+--------+
                          |                          |
                          | Download Media           | Dequeue Job
                          |                          v
                          +-----------------+ Go Worker Process
                                            +--------+--------+
                                                     |
                                                     v
                                          Platform Connectors
                                      (X, YouTube, TikTok, Instagram)
```

- **API (`cmd/api`)**: Owns authentication, input validation, post persistence, outbox event generation, scheduled post claiming, media presigned URL generation, and SSE status streaming.
- **Worker (`cmd/worker`)**: Owns platform dispatch, media retrieval, OAuth 2.0 token refresh, rate-limit handling, business retries, failure reconciliation, and terminal dead-letter logging.
- **PostgreSQL**: Authoritative store for users, accounts, posts, publishing attempts, media records, outbox leases, and failure audits.
- **Redis & Asynq**: High-throughput queueing and worker job lifecycle management.
- **Cloudflare R2 / S3**: Distributed object storage for large media assets (images, videos) supporting direct client-side presigned uploads.

---

## Key Features

- **Multi-Platform Publishing Connectors**:
  - **Twitter/X**: Text posts and image or video publishing.
  - **YouTube**: Video uploads with resumable direct publishing via YouTube Data API v3.
  - **TikTok**: Video publishing and delivery to the TikTok inbox, depending on account and app access.
  - **Instagram**: Feed photos, Reels, carousels, and Stories for eligible professional accounts. Blink prepares Instagram JPEGs and preserves original media for other destinations.
  - **Mock Mode**: Offline publishing emulator for the X connector.
- **Durable Reliability Engine**:
  - **Transactional Outbox Pattern**: Guaranteed dispatch from PostgreSQL to Redis without dual-write inconsistencies.
  - **Bounded Business Retries**: Exponential backoff with jitter and respect for platform-provided `Retry-After` headers.
  - **Ambiguous Result Reconciliation**: External timeouts or network hiccups marked `UNKNOWN` and reconciled before duplicating external posts.
  - **Dead-Letter Queue (DLQ)**: Terminal failures persisted in `dead_letter_jobs` and `publication_attempt_failures` tables for observability and replay.
  - **Automatic OAuth 2.0 Refresh**: Encrypted token storage (AES-256-GCM) with proactive token expiration checks and refresh flows.
- **Post Scheduling & Content Calendar**:
  - Schedule posts with future timestamps (`scheduled_at`).
  - Durable outbox lease mechanism prevents race conditions and duplicate claiming across distributed API nodes.
  - Reschedule or cancel scheduled posts prior to execution.
  - Dedicated calendar API and interactive frontend calendar view.
- **Real-Time Status Streaming (SSE)**:
  - `GET /api/v1/posts/{id}/status` streams real-time state transitions (`SCHEDULED` -> `QUEUED` -> `PUBLISHING` -> `PUBLISHED` / `FAILED`) directly to the browser.
  - Eliminates polling overhead while supporting automatic reconnection and fallback polling.
- **Pluggable Media Storage**:
  - Presigned direct upload workflow to Cloudflare R2 (or any S3-compatible service) bypassing server bandwidth limits.
  - Local disk storage driver for development and offline testing.
  - Supports large file uploads (up to 2 GiB) with MIME-type and size validation.
- **Operations**:
  - Clean graceful shutdown handling `SIGINT` / `SIGTERM` across both API and worker.
  - Prometheus metrics (`/metrics`) tracking publish attempts, latencies, errors, and outbox throughput.
  - Structured JSON logging with Zerolog.

---

## Repository Layout

```text
cmd/
  api/                        Go API process entrypoint
  worker/                     Asynq worker process entrypoint
internal/
  api/                        Chi HTTP routing, handlers, middleware, and services
  auth/                       JWT tokens, Argon2id passwords, AES-GCM token encryption
  config/                     Environment-backed configuration loader
  connectors/                 Social platform connector interfaces and implementations
    twitter/                  Twitter/X API v2 & v1.1 chunked media upload
    youtube/                  YouTube Data API v3 video upload
    tiktok/                   TikTok login and content posting
    instagram/                Instagram Login and container-based publishing
  jobs/                       Asynq task definitions and payload structures
  log/                        Zerolog logger initialization
  mediastore/                 Media storage drivers (Cloudflare R2 and local filesystem)
  metrics/                    Prometheus metrics declarations
  middleware/                 CORS, authentication, error recovery, request logging
  outbox/                     Outbox dispatcher and scheduled post claiming engine
  storage/                    PostgreSQL data repositories
  worker/                     Publication processor, retry logic, and DLQ handling
migrations/                   Tracked PostgreSQL schema migrations
docs/                         Operational guides and technical design documents
infra/                        Docker Compose files for PostgreSQL and Redis
web/                          Next.js web application (React 19, Tailwind CSS v4)
tests/                        Integration test suites
```

---

## Prerequisites

- **Go**: 1.26 or newer
- **Docker & Docker Compose**: For local PostgreSQL and Redis
- **PostgreSQL Client (`psql`)**: For running database migrations
- **Node.js & pnpm**: Use Node.js 24 and the pnpm version pinned in [`web/package.json`](web/package.json).
- **FFmpeg**: Install `ffprobe` when running the worker directly to validate Instagram and TikTok videos. The Docker worker includes it.

---

## Configuration

The Go processes load `.env` automatically on startup. The `Makefile` also imports and exports `.env`.

Copy the example configuration to get started:

```bash
cp .env.example .env
```

### Environment Variables

| Variable                          | Default                                            | Component      | Description                                                       |
| --------------------------------- | -------------------------------------------------- | -------------- | ----------------------------------------------------------------- |
| `PORT`                            | `8000`                                             | API            | HTTP server listening port                                        |
| `ENV`                             | `development`                                      | API / Worker   | Environment mode (`development` or `production`)                 |
| `LOG_LEVEL`                       | `info`                                             | API / Worker   | Zerolog log level (`debug`, `info`, `warn`, `error`)              |
| `DATABASE_URL`                    | `postgres://blink:devpass@localhost:5432/blink_db` | API / Worker   | PostgreSQL connection string                                      |
| `REDIS_URL`                       | `redis://localhost:6379`                           | API / Worker   | Redis connection URL for Asynq queue                              |
| `SHUTDOWN_TIMEOUT`                | `30s`                                              | API / Worker   | Graceful shutdown timeout (e.g. `10s`, `30s`, `1m`)                |
| `ENCRYPTION_KEY`                  | *required*                                         | API / Worker   | 32-byte key or 64-char hex string for AES-GCM token encryption   |
| `JWT_SIGNING_KEY_FILE` | `./.secrets/jwt-signing-key.pem` | API | Persisted Ed25519 PKCS#8 private key; provision before production startup |
| `FRONTEND_URL`                    | `http://localhost:3000`                            | API            | Frontend URL for OAuth redirects                                  |
| `CORS_ORIGINS`                    | `http://localhost:3000`                            | API            | Comma-separated list of allowed CORS origins                      |
| `PLATFORM_MODE`                   | `real`                                             | Worker         | Connector execution mode: `real` or `mock`                        |
| `TWITTER_CLIENT_ID`               | *empty*                                            | API / Worker   | Twitter/X OAuth 2.0 Client ID                                     |
| `TWITTER_CLIENT_SECRET`           | *empty*                                            | API / Worker   | Twitter/X OAuth 2.0 Client Secret                                 |
| `TWITTER_CALLBACK_URL`            | *empty*                                            | API / Worker   | Twitter/X OAuth 2.0 Redirect URI                                  |
| `YOUTUBE_CLIENT_ID`               | *empty*                                            | API / Worker   | Google / YouTube OAuth 2.0 Client ID                              |
| `YOUTUBE_CLIENT_SECRET`           | *empty*                                            | API / Worker   | Google / YouTube OAuth 2.0 Client Secret                          |
| `YOUTUBE_CALLBACK_URL`            | *empty*                                            | API / Worker   | Google / YouTube OAuth 2.0 Redirect URI                           |
| `INSTAGRAM_APP_ID`                | *empty* | API / Worker | Instagram App ID from the Instagram Login setup |
| `INSTAGRAM_APP_SECRET`            | *empty* | API / Worker | Instagram App Secret from the same setup |
| `INSTAGRAM_CALLBACK_URL`          | *empty* | API / Worker | Public HTTPS OAuth redirect ending in `/auth/instagram/callback` |
| `INSTAGRAM_API_VERSION`           | *empty* | API / Worker | Pinned Instagram Graph API version supported by the Meta app |
| `INSTAGRAM_WEBHOOK_VERIFY_TOKEN`  | *empty* | API | Optional random token shared with Meta for webhook verification |
| `META_APP_SECRET`                 | *empty* | API | Signing secret of the Meta app that owns the webhook subscription |
| `MEDIA_STORAGE_DRIVER`            | `local` (dev) / `r2` (prod)                        | API / Worker   | Storage provider (`local` or `r2`)                                |
| `MEDIA_STORAGE_PATH`              | `./media_uploads`                                  | API / Worker   | Local filesystem upload directory (when driver is `local`)        |
| `R2_ACCOUNT_ID`                   | *empty*                                            | API / Worker   | Cloudflare Account ID                                             |
| `R2_ACCESS_KEY_ID`               | *empty*                                            | API / Worker   | Cloudflare R2 API Access Key ID                                   |
| `R2_SECRET_ACCESS_KEY`           | *empty*                                            | API / Worker   | Cloudflare R2 API Secret Access Key                               |
| `R2_BUCKET_NAME`                  | `blink-media`                                      | API / Worker   | Cloudflare R2 target bucket name                                  |
| `R2_ENDPOINT`                     | *derived*                                          | API / Worker   | Custom S3 / R2 endpoint URL (defaults to Cloudflare account URL)  |
| `R2_REGION`                       | `auto`                                             | API / Worker   | S3 / R2 bucket region                                             |
| `MEDIA_MAX_UPLOAD_BYTES`          | `2147483648` (2 GiB)                               | API            | Maximum single media upload file size limit                       |
| `MEDIA_PRESIGNED_URL_TTL_SECONDS` | `900` (15 min)                                     | API            | Expiration time for client-side presigned upload URLs             |

> [!TIP]
> Generate a secure 32-byte hexadecimal encryption key with:
> ```bash
> openssl rand -hex 32
> ```

---

## Instagram Setup

Create a Meta app with Instagram Login and connect a Business or Creator account assigned to the app for testing. Blink requests `instagram_business_basic` and `instagram_business_content_publish`. Configure the four required Instagram values in the table above on both the API and worker. Use R2 for real Instagram media delivery.

Register the following URLs on the public HTTPS host that serves the Blink API.

| Meta setting | Blink endpoint |
| --- | --- |
| Business Login OAuth redirect URI | `/auth/instagram/callback` |
| Deauthorization callback | `/webhooks/instagram/deauthorize` |
| Data deletion request URL | `/webhooks/instagram/data-deletion` |
| Event webhook callback, if configured | `/webhooks/instagram` |

Generate the optional webhook verify token with `openssl rand -hex 32`, set `INSTAGRAM_WEBHOOK_VERIFY_TOKEN` and `META_APP_SECRET` on the API, then enter the same verify token under **Configure webhooks** in Meta. Leave client certificate attachment disabled unless your proxy is configured for mutual TLS. Publishing uses container polling and can run without event subscriptions.

Apply pending migrations, start the API and worker, then select **Connect Instagram** in Accounts. Use **Reconnect** for an existing connection to refresh the profile name and image while keeping its post history. See the [Instagram setup guide](docs/instagram-setup.md) for details and live verification steps.

---

## Local Setup

### 1. Start Infrastructure and Run Migrations

Start local PostgreSQL and Redis containers, then apply all database migrations:

```bash
make db-setup
```

You can also run or revert migrations manually at any time:

```bash
make migrate-up    # Apply only pending tracked migrations
make migrate-down  # Roll back ONE applied migration
```

### 2. Run API and Worker

Run the API service:

```bash
make run-api
```

In a separate terminal, start the background worker:

```bash
make run-worker
```

For offline X testing, use `PLATFORM_MODE=mock make run-worker`. Other configured providers can still publish to their live APIs.

The API will listen at `http://localhost:8000` (or the configured `PORT`).

---

## API Routes

### Public & System

| Method | Endpoint   | Description                           |
| ------ | ---------- | ------------------------------------- |
| `GET`  | `/health`  | Health check endpoint                 |
| `GET`  | `/metrics` | Prometheus metrics scrape endpoint    |

### Authentication

| Method | Endpoint       | Description                                              |
| ------ | -------------- | -------------------------------------------------------- |
| `POST` | `/auth/signup` | Register a new user account                              |
| `POST` | `/auth/login`  | Authenticate user and issue session token / cookie       |
| `POST` | `/auth/logout` | Invalidate current session and clear auth cookie          |
| `GET`  | `/auth/me`     | Return authenticated user profile (`RequireAuth`)        |

### Connected Accounts & OAuth

| Method   | Endpoint                  | Description                                                |
| -------- | ------------------------- | ---------------------------------------------------------- |
| `GET`    | `/auth/{platform}`        | Initiate OAuth for the selected provider (`RequireAuth`)               |
| `GET`    | `/auth/{platform}/callback`| OAuth 2.0 callback and token exchange                     |
| `GET`    | `/api/v1/accounts`        | List all linked social accounts for current user           |
| `DELETE` | `/api/v1/accounts/{platform}` | Disconnect a linked social account                     |

### Posts & Scheduling

| Method   | Endpoint                     | Description                                                   |
| -------- | ---------------------------- | ------------------------------------------------------------- |
| `POST`   | `/api/v1/posts`              | Create post (publish immediately or schedule with `scheduled_at`) |
| `GET`    | `/api/v1/posts`              | List posts with status, platform, and date filters            |
| `GET`    | `/api/v1/posts/upcoming`     | List scheduled upcoming posts                                 |
| `GET`    | `/api/v1/posts/{id}`         | Get details, target platform status, and attempts for a post  |
| `GET`    | `/api/v1/posts/{id}/status`  | **Server-Sent Events (SSE)** real-time status transition stream|
| `DELETE` | `/api/v1/posts/{id}`         | Delete a draft or scheduled post                              |
| `POST`   | `/api/v1/posts/{id}/retry`   | Retry a failed post publication                               |
| `PATCH`  | `/api/v1/posts/{id}/schedule`| Reschedule an existing post to a new timestamp                |
| `DELETE` | `/api/v1/posts/{id}/schedule`| Cancel a post's schedule and revert to draft                  |
| `GET`    | `/api/v1/calendar`           | Fetch posts formatted for the content calendar view           |

*(Legacy aliases under `/api/posts*` remain available for backward compatibility).*

### Media

| Method   | Endpoint                       | Description                                                  |
| -------- | ------------------------------ | ------------------------------------------------------------ |
| `POST`   | `/api/v1/media`                | Direct multipart media upload (local disk / small files)      |
| `POST`   | `/api/v1/media/uploads`        | Initiate presigned direct upload workflow (Cloudflare R2/S3) |
| `POST`   | `/api/v1/media/{id}/complete`  | Verify and finalize presigned upload after direct client put |
| `GET`    | `/api/v1/media/{id}`           | Get media record and accessible download URL                 |
| `DELETE` | `/api/v1/media/{id}`           | Delete media record and remove object from storage           |

---

## Publishing and Reliability Lifecycle

```text
       Immediate Post                              Scheduled Post
             |                                           |
             v                                           v
[ Persist Post & Outbox Event ]             [ Persist Post (scheduled_at) ]
             |                                           |
             v                                           | (Time reaches due date)
   (Outbox Dispatcher)                                   v
             |                              [ Background Outbox Lease Claim ]
             +--------------------+----------------------+
                                  |
                                  v
                       [ Enqueue to Redis/Asynq ]
                                  |
                                  v
                        [ Worker Picks Up Job ]
                                  |
               +------------------+------------------+
               |                  |                  |
               v                  v                  v
        [ Token Check ]    [ Fetch Media ]    [ Platform Publish ]
               |                  |                  |
        (Renew if needed)  (From R2 / Disk) (Selected platform)
                                                     |
                                  +------------------+------------------+
                                  |                                     |
                               Success                               Failure
                                  |                                     |
                                  v                         +-----------+-----------+
                          [ Mark PUBLISHED ]                |                       |
                                                       Retryable               Terminal
                                                            |                       |
                                                            v                       v
                                                   [ Schedule Retry ]       [ Record to DLQ ]
                                                  (Backoff / Rate Limit)    (Mark Post FAILED)
```

- **Outbox Dispatch**: Database transactions write posts and outbox events together, ensuring no message loss if Redis is temporarily unreachable.
- **Lease Claiming**: Scheduled posts are claimed with database row locks (`SKIP LOCKED`) and expiration leases so distributed API instances do not double-dispatch.
- **Bounded Retries**: Transient failures (network drops, platform `5xx`, rate limits) are retried with exponential backoff, capped attempts, and respect for `Retry-After`.
- **Reconciliation**: When an external network connection cuts out mid-request, Blink records target status as `UNKNOWN` and checks platform post existence before re-attempting.
- **Dead-Letter Storage**: Posts exceeding maximum retries or hitting fatal non-retryable errors are routed to `failed_publications` and `publication_errors`.

See the operational documentation for deeper details:
- [Publishing Reliability & Recovery](docs/publishing_reliability.md)
- [Retry Semantics & Backoff](docs/retry.md)
- [OAuth Token Refresh](docs/oauth_refresh.md)
- [Mock Connector Mode](docs/mock_mode.md)
- [Graceful Shutdown Guide](docs/graceful_shutdown.md)
- [Prometheus Metrics](docs/metrics.md)

---

## Frontend (`web`)

The web dashboard is built with Next.js 16 (App Router), React 19, Tailwind CSS v4, and TanStack React Query.

Features:
- **Post Composer**: Per-platform captions and media validation. Instagram photos can be prepared from JPEG, PNG, or WebP, with fit and crop previews when framing is needed. Reels accept one video and carousels retain the selected attachment order.
- **Post Details**: Preview the saved media for each destination, including every carousel item, alongside publishing status, links, and connected profile names and pictures when available.
- **Real-Time Updates**: Leverages Server-Sent Events (`usePostStatusStream`) to update publishing progress live without polling.
- **Content Calendar**: Month and agenda views to organize, schedule, reschedule, and delete planned social posts.
- **Media Uploader**: Client-side media validation for video duration, format, and aspect ratios, with direct upload to Cloudflare R2 via presigned URLs.
- **Account Manager**: Connect, view, and unlink social platform accounts.

### Running the Frontend

```bash
cd web
pnpm install
pnpm dev
```

The frontend will be accessible at `http://localhost:3000`. Set `NEXT_PUBLIC_API_URL` in `web/.env.local` to your backend API URL, for example `http://localhost:8000` when the API uses its default port.

---

## Testing & Quality

Run the Go test suite. Database and queue integration tests require the test services described below.

```bash
go test ./...
```

Run static analysis across core packages:

```bash
go vet ./cmd/api ./cmd/worker ./internal/config ./internal/worker ./internal/api/...
```

To run integration tests requiring live databases, set test connection strings:

```bash
BLINK_TEST_DATABASE_URL="postgres://blink:devpass@localhost:5432/blink_db?sslmode=disable" \
BLINK_TEST_REDIS_URL="redis://localhost:6379" \
go test -v ./tests/...
```

To check the frontend and Instagram composer and preview behavior:

```bash
cd web
pnpm typecheck
pnpm test:overview
pnpm test:tiktok
node test-instagram-feed-photo.mjs
node test-media-uploader.mjs
```

---

## Useful Commands

```bash
make up             # Start PostgreSQL and Redis containers in the background
make down           # Stop local infrastructure containers
make logs           # Follow container logs
make ps             # Check container health and status
make db-setup       # Start containers and apply all schema migrations
make db-reset       # Teardown volumes, restart containers, and reapply migrations
make migrate-up     # Apply only pending tracked migrations
make migrate-down   # Roll back ONE applied migration
make run-api        # Run Go API process locally
make run-worker     # Run Go Worker process locally
```

---

## Production Deployment

The [production Compose file](infra/docker-compose.prod.yml) starts PostgreSQL, Redis, the API, and the worker. Its migration service must complete successfully before the API and worker start. Deploy the Next.js frontend separately with `NEXT_PUBLIC_API_URL` set to the public API host at build time.

1. Provide the backend environment in `.env.production.local`. Set the real frontend origin in `FRONTEND_URL` and `CORS_ORIGINS`, configure R2, and persist the API signing key and token encryption key across releases.
2. Deploy the API behind an HTTPS proxy. Register production callback URLs with each provider. Omit query strings from proxy logs for OAuth callbacks, webhook verification, and signed media URLs.
3. Apply the tracked migrations before serving traffic. The production Compose migration service handles this step. Back up the database before upgrading an existing deployment.
4. Start with an Instagram tester account. On the production host, confirm OAuth, feed photos, carousels, a scheduled Reel, Stories if offered, and the deauthorization and data deletion callbacks. Confirm the event webhook separately if you enable it.
5. Before allowing customer accounts, confirm that the production Meta app has the required permissions and access level. Follow [Meta App Review](https://developers.facebook.com/docs/app-review/) for customer access.

---

## License

Blink is under active development. See [`LICENSE`](LICENSE) for licensing details.
