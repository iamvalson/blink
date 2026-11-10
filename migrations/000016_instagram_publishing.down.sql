DROP INDEX idx_social_accounts_instagram_subject;
ALTER TABLE social_accounts DROP COLUMN instagram_subject, DROP COLUMN instagram_username;
DROP TABLE instagram_deletion_receipts;
DROP TABLE instagram_publications;
DROP TABLE instagram_post_media;
ALTER TABLE post_targets DROP COLUMN instagram_format;
