-- Migration 000009_post_scheduling: Add post scheduling fields, generation tracking, and indexes.

ALTER TABLE posts
    ADD COLUMN scheduled_at TIMESTAMPTZ,
    ADD COLUMN published_at TIMESTAMPTZ,
    ADD COLUMN scheduling_generation INT NOT NULL DEFAULT 1;

ALTER TABLE posts
    DROP CONSTRAINT posts_status_check;

ALTER TABLE posts
    ADD CONSTRAINT posts_status_check CHECK (status IN (
        'DRAFT',
        'SCHEDULED',
        'QUEUED',
        'PUBLISHING',
        'PUBLISHED',
        'PARTIALLY_PUBLISHED',
        'FAILED',
        'CANCELLED'
    ));

-- Index for the dispatcher to claim due scheduled posts efficiently
CREATE INDEX idx_posts_scheduled_due
    ON posts (scheduled_at, status)
    WHERE status = 'SCHEDULED';

-- Index for user calendar date-range lookups
CREATE INDEX idx_posts_calendar_range
    ON posts (user_id, COALESCE(scheduled_at, published_at, created_at));

-- Index for upcoming scheduled posts ordered by scheduled_at
CREATE INDEX idx_posts_upcoming
    ON posts (user_id, scheduled_at ASC)
    WHERE status = 'SCHEDULED';
