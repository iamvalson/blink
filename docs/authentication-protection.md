# Authentication protection and request logging

Login and signup use shared Redis counters, bounded request bodies, and a
per-process credential concurrency limit. No database migration is needed.

## Defaults

| Route | Per-client attempt budget |
| --- | --- |
| POST /auth/login | 10 requests per minute |
| POST /auth/signup | 5 requests per 15 minutes |

The fixed window starts with the first request. Every admitted request counts,
including successful and malformed attempts. Denied requests do not extend the
window. IPv4 addresses use their own counter; IPv6 addresses share a /64 counter.
Users behind one NAT share a budget. Login and signup have separate counters.
Redis keys contain a SHA-256 digest of the client address, not email addresses
or passwords. The digest is pseudonymous, not anonymized; protect Redis access.

All API replicas must use the same Redis logical database. Lua updates the
counter and its expiration atomically. A rejected request returns 429 and a
Retry-After header in seconds. Redis failures return 503 rather than allowing
an unprotected attempt. API startup also checks Redis connectivity.

Each API process admits at most two concurrent login/signup requests combined.
Excess requests receive 503 with Retry-After: 1; they are not queued in memory.
Bodies are limited to 16 KiB even when Content-Length is absent or a valid JSON
object is followed by oversized padding. Multiple JSON values are rejected.
Auth body reads have a ten-second deadline when supported by the HTTP writer.
Credential requests have a fifteen-second context deadline. Login passwords
longer than 128 Unicode code points are rejected before password hashing.
Video uploads and SSE responses retain their existing body/streaming behavior.

## Deployment steps

1. Run the API against the intended Redis deployment. Keep the Redis database
   stable across API restarts and replicas so counters are shared. Losing or
   evicting Redis keys resets the attempt budgets; use appropriate Redis
   persistence and capacity, plus ingress abuse protection.
2. Set AUTH_TRUSTED_PROXY_CIDRS only when the API is behind a proxy. Leave it
   empty for direct connections. Use the actual proxy IP/CIDRs for your
   deployment; do not trust the entire internet or every private network.
   Invalid entries and universal /0 ranges cause startup to fail.
3. Configure that proxy to overwrite X-Forwarded-For with the observed client
   address, or append the actual connecting address to the existing chain.
   Restrict direct API ingress to that proxy. The API ignores forwarding headers
   from untrusted socket peers and walks trusted chains from right to left.
   X-Real-IP, Forwarded, and provider-specific headers are not identity sources.
   A trusted proxy that omits X-Forwarded-For makes its clients share one budget.
4. Configure proxy/CDN/access/error logs to omit query strings on OAuth callback
   and signed-media routes. The application logger now records only escaped
   URL paths, request IDs, response status, response byte counts, and duration.
   It never records request query strings, headers, or bodies. The duplicate
   Chi access logger has been removed. These changes do not erase older logs.
   Handle historical logs under your access and retention policy.
5. Restart/deploy the API. Counters persist in Redis; no worker or schema change
   is required. Confirm valid login/signup, 413 on oversized input, 429 with a
   Retry-After header after exhausting a budget, and separate budgets for
   separate real client addresses behind the deployed proxy.
6. In a test environment, stop access to Redis and confirm authentication
   returns 503. Restore Redis and retry. Do not run outage tests against an
   active publishing deployment.

These controls limit per-client abuse and local credential resource use.
Distributed attacks against an account still warrant ingress/bot protection;
this change does not introduce account lockouts or CAPTCHA.

## Verification

Run from the repository root in the Go/WSL environment:

```sh
BLINK_TEST_REDIS_URL=redis://localhost:6379 \
  go test -race ./internal/middleware ./internal/api/... ./internal/config ./cmd/api -count=1
go vet ./...
go build ./...
```

The Redis integration test uses uniquely named, short-lived keys and checks
that concurrent limiter instances share one budget, keys expire, and rejected
requests do not renew the window. Middleware tests cover spoofed forwarding
headers, IPv6 rotation, unavailable stores, and bounded credential concurrency.
Handler tests cover unknown-length bodies and oversized trailing data. Router
tests verify the actual auth routes and both request logger paths.
