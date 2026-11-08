-- Disable Inbox upload records before restoring a schema without this mode.
UPDATE posts
SET status = 'CANCELLED'
WHERE id IN (
    SELECT post_id FROM post_targets WHERE tiktok_options->>'send_to_inbox' = 'true'
);

UPDATE post_targets
SET status = 'CANCELLED'
WHERE tiktok_options->>'send_to_inbox' = 'true';

ALTER TABLE posts DROP CONSTRAINT posts_status_check;
ALTER TABLE posts ADD CONSTRAINT posts_status_check CHECK (status IN (
    'DRAFT', 'SCHEDULED', 'QUEUED', 'PUBLISHING', 'PUBLISHED', 'PARTIALLY_PUBLISHED', 'FAILED', 'CANCELLED'
));

ALTER TABLE post_targets DROP CONSTRAINT post_targets_status_check;
ALTER TABLE post_targets ADD CONSTRAINT post_targets_status_check CHECK (status IN (
    'PENDING', 'PUBLISHING', 'PUBLISHED', 'FAILED', 'CANCELLED'
));
