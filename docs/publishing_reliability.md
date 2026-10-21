# Publishing recovery

Apply all migrations, including `000010_outbox_leases`, before starting the API
and worker with this version. Deploy both processes together: the worker requires
schedule generations, and the dispatcher owns durable retry delivery.

The dispatcher claims outbox events with an expiring claim token. Temporary
Redis failures return the event to `PENDING` with backoff; expired `PROCESSING`
events are reclaimable. Enqueue uses an event-derived task ID. A malformed event
is failed individually so other events can still dispatch.

Publication workers serialize a post through a PostgreSQL session advisory lock.
The publication repository and media metadata reads reuse that session. This
requires a direct PostgreSQL connection or session pooling, not transaction-mode
connection pooling. Claiming a post checks its status, generation and due time in
one update. Cancellation and rescheduling use the same post row, so they either
win before publishing starts or return a conflict.

Retry state, failure history and a target-specific outbox event commit in one
transaction. `available_at` and `next_retry_at` enforce the retry time. Asynq retries
are for infrastructure errors; business retries remain bounded by `MaxAttempts`.
The dispatcher also checks for stalled queued/publishing posts after ten minutes
to repair lost deliveries without resetting attempts. Successful and permanently
failed targets update the parent status transactionally.

An attempt left `PROCESSING` or `UNKNOWN` is reconciled before further publishing.
When the connector cannot confirm publication, it remains `UNKNOWN` with
`MANUAL_REVIEW_REQUIRED`. It is excluded from automatic recovery and manual post
retry. There is no automatic assertion that an uncertain publication did not
happen. A dedicated operator resolution workflow is still needed.

Manual retry supports failed and partially published posts. It resets only failed
attempts and targets, increments the schedule generation, and preserves successful
publications. The attempt counter restarts for this explicit retry; the existing
failure tables retain summaries rather than a separate immutable retry-run history.

Twitter/X text publishing remains supported. Real X media uploads are rejected
until an upload implementation exists. Posts accept one durable media attachment;
multiple or conflicting media identifiers are rejected. Direct uploads check
actual streamed size and validate the stored signature, including storage read
failures.

## Tests

Unit tests do not need PostgreSQL or Redis:

```sh
go test ./internal/...
```

Integration tests use explicit test service URLs, never the application's `.env`.
Each test creates and drops its own schema, applying all migrations. The database
user needs schema creation privileges. Redis cleanup is restricted to the test's
own task IDs.

```sh
BLINK_TEST_DATABASE_URL='postgres://user:password@localhost:5432/test_db?sslmode=disable' \
BLINK_TEST_REDIS_URL='redis://localhost:6379' \
go test -race ./...
```

Without a URL, the corresponding integration tests skip. With a URL configured,
connection and migration errors fail the tests. The suite covers outbox claim
expiry, concurrent dispatch, retry transaction rollback, retry timing, exhaustion,
schedule generations, cancellation races, duplicate workers, ambiguous results,
mixed target outcomes, manual retry, and recovery after delivery loss.
