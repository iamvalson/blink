-- Existing users have no recorded agreement for this version.
ALTER TABLE users
    ADD COLUMN legal_version TEXT,
    ADD COLUMN legal_accepted_at TIMESTAMPTZ,
    ADD CONSTRAINT users_legal_acceptance_pair
        CHECK ((legal_version IS NULL) = (legal_accepted_at IS NULL));
