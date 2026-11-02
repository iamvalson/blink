ALTER TABLE post_targets ADD COLUMN tiktok_options JSONB;
CREATE TABLE tiktok_publications (
 publication_attempt_id UUID PRIMARY KEY REFERENCES publication_attempts(id) ON DELETE CASCADE,
 publish_id TEXT UNIQUE,
 remote_status TEXT NOT NULL DEFAULT 'INITIALIZING',
 poll_count INT NOT NULL DEFAULT 0 CHECK (poll_count >= 0),
 next_check_at TIMESTAMPTZ,
 deadline TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tiktok_publications_due ON tiktok_publications(next_check_at) WHERE publish_id IS NOT NULL;
