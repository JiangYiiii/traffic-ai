-- 000010: 概念归位 —— upstreams 本质是"模型账号"，统一为 model_accounts；
-- 清理从未启用的 provider_accounts 空表；usage_logs 去掉冗余的 account_id 字段。
--
-- 背景：
--   provider_accounts 表设计为"平台级账号池"，从未被使用（0 行）。
--   真正承担"模型账号"职责的是 upstreams 表。
--   为避免概念混淆，本 migration 把 upstreams 改名为 model_accounts，并把
--   provider_accounts 的 provider 字段合并到 model_accounts 自身。
--
-- 影响：
--   1. upstreams → model_accounts（含字段 account_id 删除、provider 字段新增）
--   2. token_group_upstreams → token_group_model_accounts（字段 upstream_id → model_account_id）
--   3. usage_logs: upstream_id → model_account_id；删除冗余的 account_id 字段
--   4. provider_accounts 空表 DROP
--
-- 安全性：
--   - 所有 RENAME/CHANGE COLUMN 保留原有数据
--   - provider 字段通过 models 表回填，保持可追溯
--   - down.sql 能完整还原到 000009 的状态

-- ==================== 1. 删除空的 provider_accounts ====================

DROP TABLE IF EXISTS `provider_accounts`;

-- ==================== 2. upstreams → model_accounts ====================

RENAME TABLE `upstreams` TO `model_accounts`;

-- 删掉原先指向 provider_accounts 的 account_id 字段与索引
ALTER TABLE `model_accounts` DROP INDEX `idx_account_id`;
ALTER TABLE `model_accounts` DROP COLUMN `account_id`;

-- 新增 provider 字段（从 models 表回填；保留原 models.provider 字段做向后兼容）
ALTER TABLE `model_accounts`
    ADD COLUMN `provider` VARCHAR(50) NOT NULL DEFAULT '' COMMENT 'openai | anthropic | google | azure | ...' AFTER `model_id`,
    ADD KEY `idx_provider` (`provider`);

UPDATE `model_accounts` ma
JOIN `models` m ON m.id = ma.model_id
SET ma.provider = m.provider;

-- ==================== 3. token_group_upstreams → token_group_model_accounts ====================

RENAME TABLE `token_group_upstreams` TO `token_group_model_accounts`;

ALTER TABLE `token_group_model_accounts`
    CHANGE COLUMN `upstream_id` `model_account_id` BIGINT UNSIGNED NOT NULL;

-- ==================== 4. usage_logs: upstream_id → model_account_id ====================

-- 先删掉旧索引（account_id 相关的索引需要在 DROP COLUMN 前清理）
ALTER TABLE `usage_logs` DROP INDEX `idx_usage_logs_account_id`;
ALTER TABLE `usage_logs` DROP INDEX `idx_usage_logs_account_created`;

-- 删掉冗余的 account_id 字段（之前是为了按账号聚合加的，现 model_account_id 已经具备这个语义）
ALTER TABLE `usage_logs` DROP COLUMN `account_id`;

-- 重命名 upstream_id → model_account_id（CHANGE COLUMN 保留数据）
ALTER TABLE `usage_logs`
    CHANGE COLUMN `upstream_id` `model_account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0;

-- 新建索引
CREATE INDEX `idx_usage_logs_model_account` ON `usage_logs`(`model_account_id`);
CREATE INDEX `idx_usage_logs_model_account_created` ON `usage_logs`(`model_account_id`, `created_at`);
