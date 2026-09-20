# Mock Platform Publishing Mode

This document explains how to use the mock platform connector for local development and integration testing of the Blink publishing worker.

## Overview

When developing locally or running automated end-to-end tests, making live calls to the X (Twitter) API can quickly exhaust API credits (resulting in `HTTP 402 Credits Depleted`) or hit rate limits (`HTTP 429`).

The **Mock X Connector** implements the same `connectors.PlatformConnector` interface as the production Twitter connector. When activated, it executes the entire publishing pipeline without making any outbound network requests to X.

### Pipeline Execution in Mock Mode

```text
POST /api/v1/posts (API)
  ↓
Insert post (DB: posts)
  ↓
Insert outbox event (DB: outbox_events)
  ↓
Outbox Dispatcher enqueues job
  ↓
Redis / Asynq queue
  ↓
Worker processes PublishJob
  ↓
Mock X Connector (simulates publish without network calls)
  ↓
Record publication attempt (DB: publication_attempts -> SUCCEEDED)
  ↓
Update target (DB: post_targets -> PUBLISHED)
  ↓
Update post status (DB: posts -> PUBLISHED)
```

---

## Configuration

The platform connector mode is controlled by the `PLATFORM_MODE` environment variable.

| Variable        | Values           | Default | Description                                                                        |
| :-------------- | :--------------- | :------ | :--------------------------------------------------------------------------------- |
| `PLATFORM_MODE` | `real` \| `mock` | `real`  | Selects whether the worker uses real platform connectors or local mock connectors. |

> [!IMPORTANT]
> **Production Safety**: If `PLATFORM_MODE` is absent or set to any value other than `mock` (case-insensitive), Blink **always defaults to the real production X connector**.

---

## How to Enable Mock Mode

### 1. In your `.env` file

Add or edit the following line in `.env`:

```bash
PLATFORM_MODE=mock
```

### 2. When starting the worker from the command line

Run the worker with the environment variable set inline:

```bash
PLATFORM_MODE=mock go run cmd/worker/main.go
```

When started in mock mode, the worker logs:

```
{"level":"warn","message":"PLATFORM_MODE is set to 'mock': using mock Twitter/X connector"}
```

---

## How to Return to Real Mode

### 1. In your `.env` file

Set `PLATFORM_MODE` to `real` or remove the line entirely:

```bash
PLATFORM_MODE=real
```

### 2. When starting the worker

Run the worker without `PLATFORM_MODE` (or explicitly set `PLATFORM_MODE=real`):

```bash
go run cmd/worker/main.go
# or
PLATFORM_MODE=real go run cmd/worker/main.go
```

When started in real mode, the worker logs:

```
{"level":"info","message":"PLATFORM_MODE is set to 'real' (or default): using real Twitter/X connector"}
```

---

## Verifying Publications in PostgreSQL

After creating a post and allowing the worker to process the job, you can verify the results directly in PostgreSQL.

### 1. Check Post Status

```sql
SELECT id, user_id, status, caption, created_at, updated_at
FROM posts
ORDER BY created_at DESC
LIMIT 5;
```

_Expected status_: `PUBLISHED`

### 2. Check Publication Attempts

```sql
SELECT id, post_target_id, status, platform_post_id, platform_url, error_code, started_at, completed_at
FROM publication_attempts
ORDER BY created_at DESC
LIMIT 5;
```

_Expected results in mock mode_:

- `status`: `SUCCEEDED`
- `platform_post_id`: Starts with `mock_x_` (e.g. `mock_x_a1b2c3d4e5f6`)
- `platform_url`: `http://mock.x.local/status/mock_x_...`
- `error_code`: `NULL`

### 3. Check Post Targets

```sql
SELECT id, post_id, social_account_id, status, created_at
FROM post_targets
ORDER BY created_at DESC
LIMIT 5;
```

_Expected status_: `PUBLISHED`

### 4. Full Join Verification Query

```sql
SELECT
    p.id AS post_id,
    p.status AS post_status,
    p.caption,
    sa.platform,
    pt.status AS target_status,
    pa.status AS attempt_status,
    pa.platform_post_id,
    pa.platform_url
FROM posts p
JOIN post_targets pt ON pt.post_id = p.id
JOIN social_accounts sa ON sa.id = pt.social_account_id
JOIN publication_attempts pa ON pa.post_target_id = pt.id
ORDER BY p.created_at DESC
LIMIT 5;
```
