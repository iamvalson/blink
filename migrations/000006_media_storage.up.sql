CREATE TABLE media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    content_type VARCHAR(127) NOT NULL,
    file_size BIGINT NOT NULL,
    upload_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT media_upload_status_check CHECK (upload_status IN ('pending', 'complete', 'failed')),
    CONSTRAINT media_file_size_check CHECK (file_size >= 0)
);

CREATE INDEX idx_media_user_id ON media(user_id);
CREATE INDEX idx_media_storage_key ON media(storage_key);

ALTER TABLE posts ADD COLUMN media_id UUID REFERENCES media(id) ON DELETE SET NULL;
CREATE INDEX idx_posts_media_id ON posts(media_id) WHERE media_id IS NOT NULL;

