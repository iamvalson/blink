ALTER TABLE post_targets ADD COLUMN instagram_format text
    CHECK (instagram_format IN ('IMAGE', 'REELS', 'CAROUSEL', 'STORIES'));

CREATE TABLE instagram_post_media (
    post_id uuid NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    position integer NOT NULL,
    media_id uuid REFERENCES media(id) ON DELETE SET NULL,
    PRIMARY KEY (post_id, position)
);

CREATE TABLE instagram_publications (
    attempt_id uuid PRIMARY KEY REFERENCES publication_attempts(id) ON DELETE CASCADE,
    containers text[] NOT NULL DEFAULT '{}',
    container_id text,
    submitted_at timestamptz,
    published_id text,
    deadline timestamptz NOT NULL,
    next_check_at timestamptz NOT NULL
);

CREATE TABLE instagram_deletion_receipts (
    id uuid PRIMARY KEY,
    completed_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE social_accounts ADD COLUMN instagram_subject text, ADD COLUMN instagram_username text;
CREATE INDEX idx_social_accounts_instagram_subject ON social_accounts(instagram_subject) WHERE platform = 'instagram';
