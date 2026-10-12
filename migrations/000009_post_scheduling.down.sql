-- Rollback migration 000009_post_scheduling

DROP INDEX IF EXISTS idx_posts_upcoming;
DROP INDEX IF EXISTS idx_posts_calendar_range;
DROP INDEX IF EXISTS idx_posts_scheduled_due;

ALTER TABLE posts
    DROP CONSTRAINT posts_status_check;

ALTER TABLE posts
    ADD CONSTRAINT posts_status_check CHECK (status IN (
        'DRAFT',
        'QUEUED',
        'PUBLISHING',
        'PUBLISHED',
        'PARTIALLY_PUBLISHED',
        'FAILED',
        'CANCELLED'
    ));

ALTER TABLE posts
    DROP COLUMN IF EXISTS scheduling_generation,
    DROP COLUMN IF EXISTS published_at,
    DROP COLUMN IF EXISTS scheduled_at;
