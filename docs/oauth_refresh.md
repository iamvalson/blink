# OAuth token refresh

The publish processor checks `expires_at` with a 60-second safety window before
calling a connector. Connectors own the provider-specific refresh request. The
processor encrypts the returned access token and any rotated refresh token, then
persists both with expiry and `updated_at` in one database transaction before it
starts the platform operation.

An omitted expiry is treated as provider-specific information rather than an
automatic refresh condition. X and Google return expiry data during refresh;
their connectors preserve the existing refresh token when the provider does not
return a replacement.

An authentication response can cause one refresh and one retry of the affected
operation. It never loops and is separate from the publication retry/backoff
policy. Invalid or revoked refresh tokens are permanent authentication failures;
provider/network failures remain retryable.

Refreshes for one social account are serialized inside a worker process. The
database update is atomic, but this is not a distributed lock. Deployments with
multiple worker processes should use one active publisher per account or add a
distributed account lock if the provider's refresh-token rotation semantics
require it.
