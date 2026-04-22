-- 将 traffic_ai 库补齐到当前代码所需结构（与 migrations/000002～000005 等价，可重复执行）
-- 用法: mysql -u root -h 127.0.0.1 traffic_ai < scripts/ensure_traffic_ai_schema.sql

SET NAMES utf8mb4;

-- ========== 000002: oauth_states + upstreams OAuth 列 ==========

CREATE TABLE IF NOT EXISTS `oauth_states` (
    `state`          VARCHAR(64)  NOT NULL COMMENT '随机 state 参数',
    `provider_id`    VARCHAR(50)  NOT NULL COMMENT '商家 ID（如 openai）',
    `code_verifier`  VARCHAR(128) NOT NULL COMMENT 'PKCE code_verifier',
    `redirect_info`  JSON         COMMENT '授权完成后需要的上下文信息',
    `expires_at`     DATETIME     NOT NULL COMMENT '过期时间（5分钟）',
    `created_at`     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`state`),
    KEY `idx_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'upstreams' AND COLUMN_NAME = 'auth_type';
SET @sql = IF(@col = 0,
  'ALTER TABLE `upstreams` ADD COLUMN `auth_type` VARCHAR(20) NOT NULL DEFAULT ''api_key'' COMMENT ''api_key | oauth_authorization_code'' AFTER `credential`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'upstreams' AND COLUMN_NAME = 'refresh_token';
SET @sql = IF(@col = 0,
  'ALTER TABLE `upstreams` ADD COLUMN `refresh_token` TEXT COMMENT ''AES-256 加密的 refresh_token（仅 OAuth 类型）'' AFTER `auth_type`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'upstreams' AND COLUMN_NAME = 'token_expires_at';
SET @sql = IF(@col = 0,
  'ALTER TABLE `upstreams` ADD COLUMN `token_expires_at` DATETIME DEFAULT NULL COMMENT ''access_token 过期时间'' AFTER `refresh_token`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

-- ========== 000004: models 连通性测试列（缺此列会导致 ListModels 查询失败）==========

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'models' AND COLUMN_NAME = 'last_test_ok';
SET @sql = IF(@col = 0,
  'ALTER TABLE `models` ADD COLUMN `last_test_ok` TINYINT NULL COMMENT ''NULL=未测,1=成功,0=失败'' AFTER `is_active`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'models' AND COLUMN_NAME = 'last_test_at';
SET @sql = IF(@col = 0,
  'ALTER TABLE `models` ADD COLUMN `last_test_at` DATETIME NULL AFTER `last_test_ok`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'models' AND COLUMN_NAME = 'last_test_latency_ms';
SET @sql = IF(@col = 0,
  'ALTER TABLE `models` ADD COLUMN `last_test_latency_ms` INT NULL AFTER `last_test_at`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'models' AND COLUMN_NAME = 'last_test_error';
SET @sql = IF(@col = 0,
  'ALTER TABLE `models` ADD COLUMN `last_test_error` VARCHAR(500) NULL AFTER `last_test_latency_ms`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

-- ========== 000007: models.is_listed 上架状态字段 ==========

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'models' AND COLUMN_NAME = 'is_listed';
SET @sql = IF(@col = 0,
  'ALTER TABLE `models` ADD COLUMN `is_listed` TINYINT DEFAULT 0 COMMENT ''是否上架展示给用户 0=未上架 1=已上架'' AFTER `is_active`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

-- 为现有启用模型设置为上架
UPDATE `models` SET `is_listed` = 1 WHERE `is_active` = 1 AND `is_listed` = 0;

-- ========== 000005: 账号 / 套餐 / upstreams.account_id / api_keys.key_type ==========

CREATE TABLE IF NOT EXISTS `provider_accounts` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `account_name`    VARCHAR(100) NOT NULL COMMENT '账号显示名，如 OpenAI-主力号',
    `provider`        VARCHAR(50)  NOT NULL COMMENT 'openai | anthropic | google | azure | ...',
    `auth_type`       VARCHAR(30)  NOT NULL DEFAULT 'api_key' COMMENT 'api_key | oauth',
    `credential`      TEXT         NOT NULL COMMENT 'AES-256 加密的凭证 (API Key / access_token)',
    `refresh_token`   TEXT         NULL COMMENT 'AES-256 加密, 仅 OAuth（TEXT 列不可用 DEFAULT，空用 NULL）',
    `token_expires_at` DATETIME    NULL COMMENT 'access_token 到期时间, 仅 OAuth',
    `billing_method`  VARCHAR(30)  NOT NULL DEFAULT 'postpaid' COMMENT 'prepaid | postpaid | subscription',
    `status`          VARCHAR(20)  NOT NULL DEFAULT 'online' COMMENT 'online | offline | deleted',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_provider` (`provider`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'upstreams' AND COLUMN_NAME = 'account_id';
SET @sql = IF(@col = 0,
  'ALTER TABLE `upstreams` ADD COLUMN `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''关联 provider_accounts.id'' AFTER `model_id`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @idx FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'upstreams' AND INDEX_NAME = 'idx_account_id';
SET @sql = IF(@idx = 0,
  'ALTER TABLE `upstreams` ADD KEY `idx_account_id` (`account_id`)',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

-- 套餐相关表已移除（000006迁移），用户可直接使用所有上架模型

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'api_keys' AND COLUMN_NAME = 'key_type';
SET @sql = IF(@col = 0,
  'ALTER TABLE `api_keys` ADD COLUMN `key_type` VARCHAR(30) NOT NULL DEFAULT ''standard'' COMMENT ''standard | openclaw_token'' AFTER `token_group`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

-- 默认套餐数据已移除

-- ========== 000003: endpoint 版本路径修复（幂等：仅对仍缺 /v1 的记录补全）==========

UPDATE upstreams u
JOIN models m ON u.model_id = m.id
SET u.endpoint = CONCAT(u.endpoint, '/v1')
WHERE m.provider IN ('openai', 'deepseek', 'moonshot', 'qwen', 'hunyuan', 'spark', 'baichuan', 'yi', 'minimax', 'stepfun')
  AND u.endpoint NOT LIKE '%/v1'
  AND u.endpoint NOT LIKE '%/v2'
  AND u.endpoint NOT LIKE '%/v3'
  AND u.endpoint NOT LIKE '%/v4'
  AND u.endpoint NOT LIKE '%/v1beta';

-- ========== 000013: model_accounts 连通性测试结果列 ==========

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_accounts' AND COLUMN_NAME = 'last_test_ok';
SET @sql = IF(@col = 0,
  'ALTER TABLE `model_accounts` ADD COLUMN `last_test_ok` TINYINT NULL COMMENT ''NULL=未测,1=成功,0=失败'' AFTER `timeout_sec`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_accounts' AND COLUMN_NAME = 'last_test_at';
SET @sql = IF(@col = 0,
  'ALTER TABLE `model_accounts` ADD COLUMN `last_test_at` DATETIME NULL AFTER `last_test_ok`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_accounts' AND COLUMN_NAME = 'last_test_latency_ms';
SET @sql = IF(@col = 0,
  'ALTER TABLE `model_accounts` ADD COLUMN `last_test_latency_ms` INT NULL AFTER `last_test_at`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT COUNT(*) INTO @col FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'model_accounts' AND COLUMN_NAME = 'last_test_error';
SET @sql = IF(@col = 0,
  'ALTER TABLE `model_accounts` ADD COLUMN `last_test_error` VARCHAR(500) NULL AFTER `last_test_latency_ms`',
  'SELECT 1');
PREPARE _s FROM @sql; EXECUTE _s; DEALLOCATE PREPARE _s;

SELECT 'ensure_traffic_ai_schema: done' AS status;
