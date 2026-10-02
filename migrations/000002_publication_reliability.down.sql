DROP INDEX IF EXISTS idx_publication_attempts_recovery;

ALTER TABLE publication_attempts
    DROP CONSTRAINT publication_attempts_status_check;

ALTER TABLE publication_attempts
    ADD CONSTRAINT publication_attempts_status_check CHECK (status IN (
        'PENDING',
        'PROCESSING',
        'SUCCEEDED',
        'FAILED',
        'CANCELLED'
    ));