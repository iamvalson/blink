# Password recovery with Resend

## Setup

1. Verify a sending domain in Resend and create an API key allowed to send from
   that domain. Disable click/open tracking for password-reset emails in the
   sending-domain settings. DNS verification and account limits are managed in
   Resend, not in this repository.
2. Set server-only environment variables on the API:
   ```dotenv
   RESEND_API_KEY=re_your_server_secret
   EMAIL_FROM=Blink <security@your-verified-domain.com>
   FRONTEND_URL=https://app.yourdomain.com
   CORS_ORIGINS=https://app.yourdomain.com
   ```
   Keep the existing ENCRYPTION_KEY and JWT_SIGNING_KEY_FILE persistent.
   The frontend only needs its existing NEXT_PUBLIC_API_URL; never put the
   Resend key or ENCRYPTION_KEY in a NEXT_PUBLIC variable.
3. Back up the database and apply migration 000012 with `make migrate-up`.
   Deploy all API instances with the new session-version check before enabling
   recovery. Older API instances cannot enforce revocation. The new schema is
   additive. A down migration drops recovery data and the session-version
   column; do not use it to roll back after password resets have occurred.
4. Restart/deploy the API and frontend. The API runs a small email dispatcher
   alongside the existing publishing dispatcher. A continuously running API
   process is required; request-only/serverless execution is not sufficient.
   Publishing workers need no Resend credentials or new delivery logic.
5. Visit /forgot-password using a test account you control, open the email,
   choose a new password, and log in. Confirm the old password and old sessions
   fail, replay fails, and requesting an unknown email gives the same message.
   Verify actual inbox delivery in your environment; mocked provider acceptance
   does not prove real email delivery.

Production API startup requires valid Resend configuration and a HTTPS
FRONTEND_URL. In development, omitting both mail variables disables recovery
and returns 503; no fake success or token logging is used. Partially configured
mail settings or an invalid encryption key fail startup.

## API and delivery behavior

- POST /auth/forgot-password accepts {email} and returns the same 202 message for
  known/unknown addresses. Validation failures are 400; unavailable configuration
  or storage returns 503. Email sending happens asynchronously, so provider
  latency does not expose account existence.
- POST /auth/reset-password accepts {token,password}, returning 204 on success.
  Invalid, expired, replaced, or consumed links return the same 400 error.
  Passwords follow the existing signup requirements (8-128 Unicode code points,
  uppercase/lowercase, a number and punctuation/symbol).
- Both routes use the existing Redis/IP protections, 16 KiB body limit, bounded
  concurrency and request deadlines. Forgot: 5 requests per 15 minutes per client;
  reset: 10 per 15 minutes. Trusted proxy configuration still applies.
- A database cooldown allows at most one new request per user per 60 seconds,
  including requests from different client IPs. New requests after that interval
  replace the previous link; cooldown requests retain the current link.
- Tokens use 32 random bytes, expire in 30 minutes, and are verified by SHA-256
  hash. Consumption, password hashing persistence, and incrementing the user's
  session version commit together. Concurrent submissions cannot reuse a link.
- Pending recipient/link data is AES-GCM encrypted with ENCRYPTION_KEY. It is
  removed after delivery, permanent failure, exhaustion, or expiry. Database
  backups still require restricted access; expired token hashes are inert.
- Delivery claims use row locks, SKIP LOCKED and 30-second leases. Crashes resume
  the same request with the same Resend Idempotency-Key. Transient failures retry
  after one minute, at most five delivery claims within the link's lifetime.
  Permanent provider rejections stop retries. Acceptance by Resend marks SENT;
  inbox delivery/bounces should be monitored in Resend.
- No provider response bodies, addresses, raw reset tokens or passwords are
  logged. Delivery failures log only a generic message and permanent/temporary
  classification. Inspect aggregate delivery_status counts and Resend's dashboard
  for configuration/rate-limit/delivery issues; no webhook is required.
- Password resets revoke Blink JWTs on subsequent authenticated requests.
  Existing version-zero sessions remain valid until a reset. Session validation
  reads the current version from PostgreSQL; database failure refuses access.
  Already-open SSE streams reauthenticate when they reconnect.
- Links use /reset-password#token=... so tokens do not reach frontend HTTP
  access logs or Referer headers. The page removes the fragment after capture,
  uses no-referrer/noindex metadata, and never consumes a token on a GET.
  Users who refresh before submitting can reopen the original email link.
  Exclude recovery pages from third-party analytics/session recording.

## Verification

Backend (isolated PostgreSQL schemas and uniquely named Redis test keys):

```sh
BLINK_TEST_DATABASE_URL='postgres://blink:devpass@localhost:5432/blink_db?sslmode=disable' \
BLINK_TEST_REDIS_URL=redis://localhost:6379 \
go test -race ./... -count=1
go vet ./...
go build ./...
```

Frontend, from web/: `pnpm test:recovery`, `pnpm lint`, `pnpm build`,
and `pnpm typecheck`. The focused frontend tests exercise production schemas,
fragment parsing, request contracts/error propagation and initial form rendering;
they are not a real-browser inbox test.

References: [Resend send email](https://resend.com/docs/api-reference/emails/send-email),
[verified domains](https://resend.com/docs/dashboard/domains/introduction),
[idempotency keys](https://resend.com/docs/dashboard/emails/idempotency-keys).

## Development without a sending domain

Use EMAIL_FROM=Blink <onboarding@resend.dev> and request a reset for a Blink
account registered with the same email address as the Resend account owner.
Resend restricts its test sender to that recipient; it cannot deliver recovery
emails to arbitrary users. A free Vercel frontend URL can be FRONTEND_URL, but
does not give DNS control for verifying an email-sending domain.

After fixing provider configuration, restart the API, wait at least 60 seconds
from the last reset request, and request a new link. Permanent failures are not
replayed automatically. Delivery warnings now expose only safe diagnostics:

- sender_domain_unverified: verify the EMAIL_FROM domain or use the test sender.
- testing_recipient_restricted: use the Resend account owner's email for testing.
- invalid_api_key: replace the backend Resend key.
- api_key_permission_or_domain_restriction: check the key's sending permissions.
- check_resend_dashboard: inspect the failed request in the Resend dashboard.

provider_status and provider_code identify the HTTP status and an allowlisted
error name. Raw provider messages, addresses, domains, and tokens are omitted.
