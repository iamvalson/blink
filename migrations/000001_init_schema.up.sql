CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255),
    password_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE social_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL,
    platform_user_id VARCHAR(255) NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, platform)
);

CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    caption TEXT,
    media_url VARCHAR(2048),
    media_type VARCHAR(50),
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT posts_status_check CHECK (status IN (
        'DRAFT',
        'QUEUED',
        'PUBLISHING',
        'PUBLISHED',
        'PARTIALLY_PUBLISHED',
        'FAILED',
        'CANCELLED'
    ))
);

CREATE TABLE post_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    post_id UUID NOT NULL
        REFERENCES posts(id) ON DELETE CASCADE,

    social_account_id UUID NOT NULL
        REFERENCES social_accounts(id) ON DELETE CASCADE,

    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT post_targets_status_check CHECK (
        status IN (
            'PENDING',
            'PUBLISHING',
            'PUBLISHED',
            'FAILED',
            'CANCELLED'
        )
    ),

    UNIQUE(post_id, social_account_id)
);


CREATE TABLE publication_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    post_target_id UUID NOT NULL REFERENCES post_targets(id) ON DELETE CASCADE,

    status TEXT NOT NULL DEFAULT 'PENDING',

    attempt_count INT NOT NULL DEFAULT 0,

    platform_post_id VARCHAR(255),
    platform_url VARCHAR(2048),

    error_code TEXT,
    error_message   TEXT,

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),


    CONSTRAINT publication_attempts_status_check CHECK (status IN (
        'PENDING',
        'PROCESSING',
        'SUCCEEDED',
        'FAILED',
        'CANCELLED'
    ))
);


CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    key VARCHAR(255) NOT NULL,

    request_hash VARCHAR(64) NOT NULL,

    status VARCHAR(50) NOT NULL DEFAULT 'PROCESSING',

    resource_type VARCHAR(50),
    resource_id UUID,


    response_status INT,
    response_body JSONB,

    expires_at TIMESTAMPTZ NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),



    CONSTRAINT idempotency_keys_status_check CHECK (status IN (
        'PROCESSING',
        'COMPLETED',
        'FAILED'
    )),

    CONSTRAINT idempotency_keys_key_not_empty CHECK (length(trim(key)) > 0),
    CONSTRAINT idempotency_keys_request_hash_not_empty CHECK (length(trim(request_hash)) > 0),
    CONSTRAINT idempotency_keys_response_status_check CHECK (
        response_status IS NULL OR response_status BETWEEN 100 AND 599 
    ),

    UNIQUE(user_id, key)

);


CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,

    event_type VARCHAR(100) NOT NULL,

    payload JSONB NOT NULL,

    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',

    attempts INT NOT NULL DEFAULT 0,

    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    published_at TIMESTAMPTZ,

    CONSTRAINT outbox_events_status_check
        CHECK (status IN (
            'PENDING',
            'PROCESSING',
            'PUBLISHED',
            'FAILED'
        )),

    CONSTRAINT outbox_events_attempts_check
        CHECK (attempts >= 0)
);


CREATE TABLE publish_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    publication_attempt_id UUID NOT NULL
        REFERENCES publication_attempts(id) ON DELETE CASCADE,

    post_target_id UUID NOT NULL
        REFERENCES post_targets(id) ON DELETE CASCADE,

    platform_post_id VARCHAR(255),
    published_url VARCHAR(2048),

    status VARCHAR(50) NOT NULL,

    error_code TEXT,
    error_message TEXT,

    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT publish_results_status_check
        CHECK (status IN ('SUCCEEDED', 'FAILED'))
);


CREATE INDEX idx_social_accounts_user_id
ON social_accounts(user_id);

CREATE INDEX idx_posts_user_id
ON posts(user_id);

CREATE INDEX idx_post_targets_post_id
ON post_targets(post_id);

CREATE INDEX idx_post_targets_social_account_id
ON post_targets(social_account_id);

CREATE INDEX idx_publication_attempts_post_target_id
ON publication_attempts(post_target_id);

CREATE INDEX idx_publish_results_post_target_id
ON publish_results(post_target_id);

CREATE INDEX idx_publish_results_publication_attempt_id
ON publish_results(publication_attempt_id);

CREATE UNIQUE INDEX idx_publish_results_attempt_unique
ON publish_results(publication_attempt_id);

CREATE INDEX idx_outbox_events_pending
ON outbox_events(status, available_at)
WHERE status = 'PENDING';

CREATE INDEX idx_idempotency_keys_expires_at
ON idempotency_keys(expires_at);

CREATE UNIQUE INDEX idx_publication_attempts_post_target_unique
ON publication_attempts(post_target_id);