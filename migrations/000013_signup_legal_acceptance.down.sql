ALTER TABLE users
    DROP CONSTRAINT users_legal_acceptance_pair,
    DROP COLUMN legal_accepted_at,
    DROP COLUMN legal_version;
