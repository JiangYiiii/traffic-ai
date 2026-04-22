-- 000010 down: 还原到 000009 结束时的状态
--
-- 执行顺序与 up.sql 相反：
--   1. usage_logs: model_account_id → upstream_id，加回 account_id
--   2. token_group_model_accounts → token_group_upstreams
--   3. model_accounts → upstreams（加回 account_id 字段，删掉 provider 字段）
--   4. 重建空的 provider_accounts 表
--
-- 注意：回滚不会恢复 provider_accounts 中的历史数据（原表就是空的）。

-- ==================== 4. 还原 usage_logs ====================

DROP INDEX `idx_usage_logs_model_account_created` ON `usage_logs`;
DROP INDEX `idx_usage_logs_model_account` ON `usage_logs`;

ALTER TABLE `usage_logs`
    CHANGE COLUMN `model_account_id` `upstream_id` BIGINT UNSIGNED NOT NULL DEFAULT 0;

ALTER TABLE `usage_logs`
    ADD COLUMN `account_id` BIGINT NOT NULL DEFAULT 0 AFTER `upstream_id`;

CREATE INDEX `idx_usage_logs_account_id` ON `usage_logs`(`account_id`);
CREATE INDEX `idx_usage_logs_account_created` ON `usage_logs`(`account_id`, `created_at`);

-- ==================== 3. 还原 token_group_upstreams ====================

ALTER TABLE `token_group_model_accounts`
    CHANGE COLUMN `model_account_id` `upstream_id` BIGINT UNSIGNED NOT NULL;

RENAME TABLE `token_group_model_accounts` TO `token_group_upstreams`;

-- ==================== 2. 还原 upstreams ====================

ALTER TABLE `model_accounts`
    DROP INDEX `idx_provider`,
    DROP COLUMN `provider`;

ALTER TABLE `model_accounts`
    ADD COLUMN `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '关联 provider_accounts.id' AFTER `model_id`,
    ADD KEY `idx_account_id` (`account_id`);

RENAME TABLE `model_accounts` TO `upstreams`;

-- ==================== 1. 重建空的 provider_accounts ====================

CREATE TABLE IF NOT EXISTS `provider_accounts` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `account_name`    VARCHAR(100) NOT NULL COMMENT '账号显示名',
    `provider`        VARCHAR(50)  NOT NULL COMMENT 'openai | anthropic | google | azure | ...',
    `auth_type`       VARCHAR(30)  NOT NULL DEFAULT 'api_key',
    `credential`      TEXT         NOT NULL,
    `refresh_token`   TEXT         NULL,
    `token_expires_at` DATETIME    NULL,
    `billing_method`  VARCHAR(30)  NOT NULL DEFAULT 'postpaid',
    `status`          VARCHAR(20)  NOT NULL DEFAULT 'online',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_provider` (`provider`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
