CREATE TABLE publication_attempt_failures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    publication_attempt_id UUID NOT NULL REFERENCES publication_attempts(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL CHECK (attempt_number > 0),
    error_code TEXT NOT NULL,
    error_class TEXT NOT NULL,
    error_message TEXT NOT NULL,
    platform_response JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(publication_attempt_id, attempt_number)
);

CREATE INDEX idx_publication_attempt_failures_attempt
    ON publication_attempt_failures(publication_attempt_id, occurred_at);

CREATE TABLE dead_letter_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    post_target_id UUID NOT NULL REFERENCES post_targets(id) ON DELETE CASCADE,
    job_id TEXT,
    task_type TEXT NOT NULL,
    platform VARCHAR(50) NOT NULL,
    attempts INT NOT NULL CHECK (attempts > 0),
    max_attempts INT NOT NULL CHECK (max_attempts > 0),
    failure_reason TEXT NOT NULL,
    failure_type TEXT NOT NULL CHECK (failure_type IN (
        'AUTHENTICATION', 'AUTHORIZATION', 'RATE_LIMIT', 'VALIDATION',
        'PLATFORM_ERROR', 'NETWORK', 'TIMEOUT', 'INTERNAL', 'UNKNOWN'
    )),
    platform_response JSONB,
    first_failed_at TIMESTAMPTZ NOT NULL,
    last_failed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(post_target_id)
);

CREATE INDEX idx_dead_letter_jobs_failure_time
    ON dead_letter_jobs(last_failed_at DESC);
CREATE INDEX idx_dead_letter_jobs_platform
    ON dead_letter_jobs(platform);
CREATE INDEX idx_dead_letter_jobs_post
    ON dead_letter_jobs(post_id);