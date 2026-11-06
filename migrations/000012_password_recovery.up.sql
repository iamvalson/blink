ALTER TABLE users ADD COLUMN session_version BIGINT NOT NULL DEFAULT 0 CHECK (session_version >= 0);

-- One active request per user bounds storage and provides a mailbox cooldown.
-- Only the verifier hash is retained after delivery. The pending email payload
-- is AES-GCM encrypted with the application's encryption key.
CREATE TABLE password_resets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    id UUID NOT NULL UNIQUE,
    token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at TIMESTAMPTZ NOT NULL,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consumed_at TIMESTAMPTZ,
    encrypted_email TEXT,
    delivery_status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (delivery_status IN ('PENDING','SENDING','SENT','FAILED','CONSUMED')),
    delivery_attempts INT NOT NULL DEFAULT 0 CHECK (delivery_attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    provider_id TEXT
);
CREATE INDEX password_resets_delivery ON password_resets(next_attempt_at)
    WHERE delivery_status IN ('PENDING','SENDING');
