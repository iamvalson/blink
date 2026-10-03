# Prometheus metrics

Blink exposes Prometheus exposition data at `GET /metrics` on the API server. The endpoint uses the default Prometheus registry and is intended for internal monitoring access.

## Metrics

- `blink_jobs_processed_total{platform}` counts publish executions started by the worker.
- `blink_jobs_succeeded_total{platform}` counts executions that publish successfully.
- `blink_jobs_failed_total{platform}` counts executions that fail at the platform publish operation.
- `blink_jobs_retried_total{platform}` counts retries after the existing retry task is successfully scheduled.
- `blink_jobs_permanently_failed_total{platform}` counts failures successfully recorded in the existing dead-letter workflow.
- `blink_publish_duration_seconds{platform}` measures each external connector publish call.
- `blink_rate_limit_events_total{platform}` counts classified platform rate-limit errors.
- `blink_token_refreshes_total{platform,result}` counts actual OAuth refresh calls, where `result` is `success` or `failure`.
- `blink_queue_failures_total{reason}` counts queue infrastructure failures. `reason` is one of `enqueue`, `processing`, `deserialization`, `handler`, `scheduling`, or `unknown`.
- `blink_worker_jobs_in_progress` reports active publish executions.

The only platform label values emitted by the metrics helpers are `twitter`, `youtube`, and `unknown`. Job IDs, post IDs, user IDs, URLs, and raw error messages remain in structured logs and are never metric labels.

## Local verification

Start the API with the repository's normal configuration, then request:

```bash
curl http://localhost:8080/metrics
```

The port is controlled by the API configuration. A publish worker execution will populate the job, latency, retry, rate-limit, and OAuth metrics as applicable. Queue depth is intentionally not exported because the current architecture has no cheap, reliable measurement for it.
