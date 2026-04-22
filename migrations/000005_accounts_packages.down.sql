-- 000005 rollback
ALTER TABLE `api_keys` DROP COLUMN `key_type`;
DROP TABLE IF EXISTS `user_packages`;
DROP TABLE IF EXISTS `package_models`;
DROP TABLE IF EXISTS `packages`;
ALTER TABLE `upstreams` DROP KEY `idx_account_id`;
ALTER TABLE `upstreams` DROP COLUMN `account_id`;
DROP TABLE IF EXISTS `provider_accounts`;
