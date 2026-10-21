DROP INDEX IF EXISTS idx_outbox_expired_leases;
DROP INDEX IF EXISTS idx_outbox_dispatchable;

ALTER TABLE outbox_events
    DROP COLUMN last_error,
    DROP COLUMN locked_until,
    DROP COLUMN claim_token;