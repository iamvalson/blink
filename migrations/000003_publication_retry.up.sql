ALTER TABLE publication_attempts
    DROP CONSTRAINT publication_attempts_status_check;

ALTER TABLE publication_attempts
    ADD COLUMN error_class TEXT,
    ADD COLUMN next_retry_at TIMESTAMPTZ;

ALTER TABLE publication_attempts
    ADD CONSTRAINT publication_attempts_status_check CHECK (status IN (
        'PENDING', 'PROCESSING', 'RETRYING', 'UNKNOWN', 'SUCCEEDED', 'FAILED', 'CANCELLED'
    ));

CREATE INDEX IF NOT EXISTS idx_publication_attempts_retry
    ON publication_attempts(status, next_retry_at)
    WHERE status = 'RETRYING';