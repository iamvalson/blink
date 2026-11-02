# Phase 3: approved TikTok URL delivery design

Keep R2 private. A dedicated verified HTTPS hostname routes to Blink's API. HMAC-signed URLs bind one media UUID and expiry, permit GET/HEAD only, and expire after two hours. Mint at execution, not scheduling. TikTok downloads through Blink; do not redirect to R2 or substitute a presigned S3 hostname. Application URL logging is bypassed for delivery. Proxy logs must redact queries; caches and challenges must be bypassed. This adds API bandwidth, TLS and availability requirements. Anyone holding a link can fetch that single video until expiry; the rest of the bucket remains private.

No suitable custom-domain setting was found in repository/local configuration. Actual Cloudflare DNS/dashboard state has not been inspected. Manual setup is required, documented in the deployment guide. No domain is assumed verified merely because it is configured.

Use an optional async connector capability. Persist initialization intent before the external call, then immediately save publish_id and the first polling intent in PostgreSQL. Known IDs only poll; uncertain initialization becomes UNKNOWN/manual review. The existing outbox, locks, TokenManager and scheduling generation remain authoritative.

Interview rationale: temporary object-specific delivery preserves storage privacy and avoids a second media storage architecture. Separating initialization from polling makes accepted operations recoverable across crashes.
