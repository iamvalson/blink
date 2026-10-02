DROP INDEX IF EXISTS idx_publication_attempts_retry;
ALTER TABLE publication_attempts DROP CONSTRAINT publication_attempts_status_check;
ALTER TABLE publication_attempts DROP COLUMN error_class, DROP COLUMN next_retry_at;
ALTER TABLE publication_attempts ADD CONSTRAINT publication_attempts_status_check CHECK (status IN (
    'PENDING', 'PROCESSING', 'UNKNOWN', 'SUCCEEDED', 'FAILED', 'CANCELLED'
));