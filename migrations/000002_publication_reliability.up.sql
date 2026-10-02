ALTER TABLE publication_attempts
    DROP CONSTRAINT publication_attempts_status_check;

ALTER TABLE publication_attempts
    ADD CONSTRAINT publication_attempts_status_check CHECK (status IN (
        'PENDING',
        'PROCESSING',
        'UNKNOWN',
        'SUCCEEDED',
        'FAILED',
        'CANCELLED'
    ));

CREATE INDEX IF NOT EXISTS idx_publication_attempts_recovery
    ON publication_attempts(status, updated_at)
    WHERE status IN ('PROCESSING', 'UNKNOWN');