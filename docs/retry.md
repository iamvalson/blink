# Publication retries

Blink uses bounded retries with exponential backoff and platform reconciliation for ambiguous external operations.

An attempt is one delivery call claimed for a platform. `attempt_count` is zero before the first platform call and is incremented when that call begins. Blink permits three total delivery attempts, so the third failure is terminal and no fourth platform call is made.

Retryable failures use this asynchronous schedule:

- after attempt 1: 1 minute
- after attempt 2: 5 minutes

No calculated delay exceeds one hour. The maximum delay does not extend the three-attempt limit.

Connectors normalize platform responses into retryable, permanent, or ambiguous errors. Transient network/platform failures and HTTP 429/5xx responses are retryable. Validation, authorization, unsupported media, and other definitive 4xx failures are permanent. A timeout or lost response that may have reached the platform is ambiguous.

Ambiguous outcomes remain durable as `UNKNOWN` and go through the existing reconciliation path before another publish is considered. A found publication completes the original attempt; confirmed absence is the only reconciliation result that permits publishing again. An unresolved result is never blindly republished.

Retry state is persisted as `RETRYING` with `error_class` and `next_retry_at`. The same `publication_attempt_id` is used for every delivery. Asynq schedules the next execution with `ProcessIn`, while `MaxRetry(0)` prevents Asynq's native retry counter from multiplying Blink's business attempts. Workers do not sleep while waiting.