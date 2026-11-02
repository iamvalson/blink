# TikTok publishing delivery report

Implemented on `dev`; no commit, push or merge. Existing untracked `docs/phase-2-content-operations-plan.txt` was preserved. Development configuration and ignored ownership/signing files were added during setup. No secrets are included in the reviewed source files. The user completed TikTok dashboard verification and removed the misrouted webhook.

## Result and architecture

Blink now supports TikTok Direct Post using PULL_FROM_URL: compose/select account and creator options, preview and consent, schedule through the existing infrastructure, validate stored video and current creator permissions, initialize, immediately persist publish_id, poll durably and finalize only on PUBLISH_COMPLETE. Public post IDs remain separate from publish_id. Private completion requires no public ID; no TikTok URL is fabricated.

Cloudflare R2 remains private and unchanged. New object-specific HMAC URLs stream video through Blink's API on a configured verified HTTPS domain/prefix. Default lifetime is two hours, minted at execution. Domain ownership verification is manual. The URL is never redirected to an unverified R2 presigned hostname. Signed delivery bypasses application request-URI logging, with GET/HEAD and single-range support, no caching and bounded-memory streaming. Infrastructure logs/caches still need manual configuration.

TikTok implements an optional AsyncVideoPublisher capability. X/YouTube upload implementations are unchanged. Shared TokenManager, post advisory locks, scheduling generations and publication attempt states remain in use. Initialization intent is written before the external call. A known ID only resumes polling; an accepted initialization with a lost ID becomes UNKNOWN/manual review. Next-check intent and the saved ID are transactional with the outbox; `tiktok:status` jobs carry post/target/attempt/generation. Existing stalled-post recovery repairs lost queue deliveries. Terminal remote status is saved before final Blink completion. Polling has a configurable deadline and a 3000-check ceiling.

Migration `000011_tiktok_publishing` adds `post_targets.tiktok_options` and `tiktok_publications`; up/down scripts are included. Existing platform post IDs and attempt-state constraints are not repurposed.

## Endpoints

| Method | Endpoint | Result |
| --- | --- | --- |
| POST | TikTok `/v2/post/publish/creator_info/query/` | Existing API reused for current options and execution-time validation |
| POST | TikTok `/v2/post/publish/video/init/` | New Direct Post initialization with PULL_FROM_URL |
| POST | TikTok `/v2/post/publish/status/fetch/` | New durable status polling/reconciliation |
| GET/HEAD | Blink `/tiktok-media/{mediaID}?expires=...&signature=...` | New signed delivery of one completed video |
| GET | Blink `/api/v1/accounts/{accountID}/tiktok/creator-info` | Existing authenticated route reused |

## Frontend

Added creator-info loading, account selection, explicit privacy choice, initially disabled interactions, disclosure controls, AI disclosure, policy links and publishing consent. Publish-now and scheduling share TikTok validation and persisted options. Backend probes actual media using ffprobe instead of trusting client metadata. The worker Docker image now packages FFmpeg/ffprobe, and stage-by-stage worker logs and frontend retry reasons expose validation and platform failures. Progress distinguishes processing, publication, failure and review; mixed-platform results can retry failed targets without resubmitting successful ones. Small existing composer/schedule lint issues were corrected while validating these paths.

## Validation

- `go build ./...` and `go vet ./...`: passed.
- `go test ./... -count=1` with `BLINK_TEST_DATABASE_URL` pointing to the local PostgreSQL test service: passed, including isolated-schema migration/integration tests. Credential-gated live-platform tests are not claimed.
- Backend coverage includes verified URL boundaries, initialization JSON/error classification, large numeric public IDs, signed GET/HEAD/Range authorization, processing/confirmed/private completion, terminal rejection, token refresh, lost response/failed ID save, interrupted preflight, restart recovery, confirmation DB failure, rate limits/deadlines, atomic outbox rollback, lost queue delivery, scheduling/options idempotency and independent X/TikTok targets.
- `pnpm lint`: passed with five existing warnings in unchanged account/media/preview/list files.
- `pnpm build` and `pnpm typecheck`: passed.
- `pnpm test:tiktok`: 25 production-validator/rendered-component checks passed (10 validation, 8 settings, 5 status, 2 retry-diagnostic checks). Tests transpile the actual production modules and do not require native TypeScript execution.
- `node test-video-validation.mjs`: existing 10 checks passed; this legacy script mirrors helper logic and is not described as end-to-end coverage.
- `go test -race ./internal/auth -count=1`: passed, including concurrent signing-key initialization.
- All four Docker targets built successfully. The worker image generated and probed a real H.264 MP4 as UID 10001; the probe returned the expected codec, dimensions, frame rate and duration.
- A temporary Docker volume preserved a mode-0600 key owned by UID/GID 10001, the API image could read it through a read-only mount, and repeat key generation exited nonzero without replacement. The temporary volume was removed after the test.
- The migration image connected to local PostgreSQL and reported version 11, dirty false, latest 11.
- `git diff --check`: passed.

PostgreSQL tests use private schemas, preserve their search_path and clean them up. The application database has now been adopted into clean tracked version 11 after a full reference-schema comparison. A missing publication_attempts_status_check was first restored after confirming every stored state was valid; no post data was rewritten. Repeated migrate-up runs then made no changes.

## Interview explanations by phase

| Phase | Choice and reason |
| --- | --- |
| 3 | A verified API delivery hostname with short-lived object signatures preserves bucket privacy without replacing R2. |
| 4 | Typed post/source/status models keep TikTok field names and creator choices explicit. |
| 5 | Initialization returns an operation ID, not success; network uncertainty is classified conservatively to prevent duplicates. |
| 6 | Following the approved scope change, URL delivery replaces chunk upload entirely; there is no unnecessary FILE_UPLOAD implementation. |
| 7 | Reuse R2's existing Get interface and the original upload paths so TikTok cannot change how X/YouTube receive media. |
| 8 | PostgreSQL/outbox owns polling intent so process restarts and Redis delivery loss do not lose the operation. |
| 9 | The initialization marker separates safe preflight recovery from an uncertain external side effect; known IDs reconcile rather than republish. |
| 10 | Register an optional async capability in the existing worker, reusing token coordination and locking. |
| 11 | Persist explicit settings against the selected account and revalidate at execution, because scheduled permissions can change. |
| 12 | Exercise external-call boundaries and real database transactions; acceptance, persistence and confirmed publication can fail independently. |
| 13 | Live development publishing, hashtags and scheduling are now verified. Recovery/load and production public-post validation remain separate checks. |
| 14 | Build, vet, unit/integration tests, lint, type checking and a production frontend build verify the delivered changes at their relevant layers. |

## Confirmed live tests (2026-11-02)

- The ngrok media URL prefix was verified with TikTok using its exact public signature file; localhost and ngrok returned HTTP 200 and identical file bytes.
- The user connected TikTok, published to TikTok and YouTube, and tested hashtags and scheduling.
- PostgreSQL independently confirmed post `95f37172-d0ae-4887-a1eb-85cad4e2992f` as PUBLISHED, both platform targets PUBLISHED and attempts SUCCEEDED, with TikTok PUBLISH_COMPLETE.
- The missing ffprobe dependency was diagnosed from persisted retry state, installed in WSL and tested against a real generated MP4. It is now included in the worker runtime image.
- The unaudited-app/private-account rejection was encountered and corrected during live testing. The user removed webhook delivery incorrectly configured against the GET OAuth callback; polling remains the implemented status path.
- Reschedule actions were added to Posts and Calendar and verified by frontend checks plus the scheduling-generation integration test. Live rescheduling is not claimed until confirmed by the user.

## Deployment readiness changes and diff review

Read [deployment-readiness instructions](deployment-readiness.md) and [TikTok deployment instructions](tiktok-publishing-deployment.md).

- Added API, worker, migration and key-generation Docker targets with CA certificates, timezone data and nonroot UID/GID 10001. Worker includes FFmpeg; private files and local ownership files are excluded from Git/image contexts.
- Replaced per-start random JWT keys with one persistent Ed25519 PKCS#8 file. Development creates it once; production requires an existing file. Public keys are derived, concurrent initialization is atomic and existing keys are never overwritten. A private ignored development key was provisioned. Restart/replica persistence, generation races, invalid keys and production refusal are tested.
- Added a pinned golang-migrate v4.20.1 runner with embedded SQL, version/dirty tracking, locks, status, one-step rollback and guarded adoption. Baseline compares columns/defaults, constraints and indexes against a rollback-only reference schema before recording the version. It refuses drift, wrong versions and existing/dirty history. Tests cover no replay, one-step down/up, adoption, failed-transaction/dirty state and concurrent commands.
- Updated Makefile, README and CI to use tracked migrations; no sslmode override is appended to deployment URLs. API/worker startup failures now exit nonzero for process supervisors.
- Reviewed the complete pending tracked diff and added files, including signed-media authorization, public verification scope, external-call ambiguity, durable outbox intent, target/attempt/generation checks, SQL state transitions, composer consent/options and cache refresh after rescheduling. No unresolved blocking code findings remain in the requested changes. Existing five frontend lint warnings remain.
- Review found and fixed the missing local database status constraint, overly quiet async diagnostics, deployment-key volatility and migration replay. Transitive module updates required by the pinned migration dependency were included and validated by the full Go suite.

One sign-in is expected when first switching away from the old ephemeral API key. Later restarts preserve unexpired sessions when they mount the same persisted key. Production replicas still need shared/sticky OAuth transaction state; the current OAuth redirect store is process-local. The existing manual-review policy for UNKNOWN attempts remains; no unsafe automatic resolution endpoint was added.

Remaining: actual production DNS/TLS/proxy configuration and public TikTok audit/review; real-platform crash/lost-queue recovery and representative load; live rescheduling confirmation; secret storage/backups and controlled deployment operations. Current live private publishing success does not establish public production approval. CI workflow changes were validated locally through their component commands; a hosted GitHub Actions run has not been claimed.

To test end-to-end: retain the verified domain and runtime secrets, run tracked migrations before deployment, mount the persisted JWT key, start the API/worker, connect the permitted TikTok account, upload one supported video, choose returned privacy/interactions/disclosures, preview and consent, publish or schedule, and confirm saved operation plus final platform state. Use the linked checklist for failure tests; do not clear uncertain operations before reviewing TikTok.

## Files created or modified

- `.dockerignore`
- `.env.example`
- `.github/workflows/ci.yml`
- `.gitignore`
- `Makefile`
- `README.md`
- `cmd/api/main.go`
- `cmd/keygen/main.go`
- `cmd/migrate/main.go`
- `cmd/worker/main.go`
- `docs/deployment-readiness.md`
- `docs/tiktok-publishing-delivery-report.md`
- `docs/tiktok-publishing-deployment.md`
- `docs/tiktok-publishing-implementation-note.md`
- `docs/tiktok-pull-delivery-design.md`
- `go.mod`
- `go.sum`
- `infra/Dockerfile`
- `internal/api/handler/tiktok_delivery.go`
- `internal/api/handler/tiktok_delivery_test.go`
- `internal/api/handler/tiktok_verification.go`
- `internal/api/handler/tiktok_verification_test.go`
- `internal/api/service/post.go`
- `internal/auth/signing_keys.go`
- `internal/auth/signing_keys_test.go`
- `internal/config/config.go`
- `internal/connectors/async.go`
- `internal/connectors/tiktok/connector.go`
- `internal/connectors/tiktok/publishing.go`
- `internal/connectors/tiktok/publishing_test.go`
- `internal/connectors/tiktok/types.go`
- `internal/jobs/publish_job.go`
- `internal/jobs/types.go`
- `internal/mediastore/delivery.go`
- `internal/mediastore/delivery_test.go`
- `internal/migration/baseline.go`
- `internal/migration/runner.go`
- `internal/model/async_publication.go`
- `internal/model/post.go`
- `internal/model/tiktok.go`
- `internal/outbox/dispatcher.go`
- `internal/outbox/dispatcher_test.go`
- `internal/storage/post.go`
- `internal/storage/tiktok_publication.go`
- `internal/worker/async_publish.go`
- `internal/worker/async_publish_test.go`
- `internal/worker/publish_processor.go`
- `internal/worker/video_probe.go`
- `internal/worker/video_probe_test.go`
- `migrations/000011_tiktok_publishing.down.sql`
- `migrations/000011_tiktok_publishing.up.sql`
- `migrations/embed.go`
- `tests/migrations_integration_test.go`
- `tests/testdb_test.go`
- `tests/tiktok_integration_test.go`
- `web/app/(dashboard)/calendar/page.tsx`
- `web/components/posts/post-composer.tsx`
- `web/components/posts/publishing-progress.tsx`
- `web/components/posts/reschedule-post-dialog.tsx`
- `web/components/posts/schedule-dialog.tsx`
- `web/components/posts/tiktok-settings.tsx`
- `web/features/posts/api/tiktok-creator-info.ts`
- `web/features/posts/components/posts-list.tsx`
- `web/features/posts/utils/tiktok-validator.ts`
- `web/package.json`
- `web/test-tiktok-ui.mjs`
- `web/test-tiktok-validation.mjs`
- `web/types/post.ts`
