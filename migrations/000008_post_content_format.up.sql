-- Add content_format to posts to distinguish YouTube Videos and Shorts.
-- VIDEO is the default to preserve backward compatibility for all existing posts.
ALTER TABLE posts
    ADD COLUMN content_format VARCHAR(50) NOT NULL DEFAULT 'VIDEO';

ALTER TABLE posts
    ADD CONSTRAINT posts_content_format_check CHECK (
        content_format IN ('VIDEO', 'SHORT')
    );

CREATE INDEX idx_posts_content_format ON posts(content_format);
