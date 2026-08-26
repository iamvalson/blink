DROP INDEX IF EXISTS idx_publish_results_post_target_id;
DROP INDEX IF EXISTS idx_post_targets_post_id;
DROP INDEX IF EXISTS idx_posts_user_id;
DROP INDEX IF EXISTS idx_social_accounts_user_id;

DROP TABLE IF EXISTS publish_results;
DROP TABLE IF EXISTS post_targets;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS social_accounts;
DROP TABLE IF EXISTS users;