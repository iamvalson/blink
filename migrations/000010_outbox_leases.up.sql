ALTER TABLE outbox_events
    ADD COLUMN claim_token UUID,
    ADD COLUMN locked_until TIMESTAMPTZ,
    ADD COLUMN last_error TEXT;

CREATE INDEX idx_outbox_dispatchable
    ON outbox_events (available_at, created_at)
    WHERE status = 'PENDING';

CREATE INDEX idx_outbox_expired_leases
    ON outbox_events (locked_until)
    WHERE status = 'PROCESSING';

