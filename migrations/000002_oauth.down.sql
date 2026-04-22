ALTER TABLE `upstreams`
    DROP COLUMN `auth_type`,
    DROP COLUMN `refresh_token`,
    DROP COLUMN `token_expires_at`;

DROP TABLE IF EXISTS `oauth_states`;
