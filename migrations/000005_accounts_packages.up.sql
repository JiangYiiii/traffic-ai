-- 000005: 模型·账号拆分 + 套餐体系 + API Key 类型
-- 变更内容:
--   1. 新增 provider_accounts 表 (提供商账号)
--   2. upstreams 表新增 account_id 外键
--   3. models 表去除 provider 字段 (provider 下沉到账号层)
--   4. 新增 packages / package_models / user_packages 表 (套餐体系)
--   5. api_keys 表新增 key_type 字段

-- ==================== 提供商账号 ====================

CREATE TABLE IF NOT EXISTS `provider_accounts` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `account_name`    VARCHAR(100) NOT NULL COMMENT '账号显示名，如 OpenAI-主力号',
    `provider`        VARCHAR(50)  NOT NULL COMMENT 'openai | anthropic | google | azure | ...',
    `auth_type`       VARCHAR(30)  NOT NULL DEFAULT 'api_key' COMMENT 'api_key | oauth',
    `credential`      TEXT         NOT NULL COMMENT 'AES-256 加密的凭证 (API Key / access_token)',
    `refresh_token`   TEXT         NULL COMMENT 'AES-256 加密, 仅 OAuth',
    `token_expires_at` DATETIME    NULL COMMENT 'access_token 到期时间, 仅 OAuth',
    `billing_method`  VARCHAR(30)  NOT NULL DEFAULT 'postpaid' COMMENT 'prepaid | postpaid | subscription',
    `status`          VARCHAR(20)  NOT NULL DEFAULT 'online' COMMENT 'online | offline | deleted',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_provider` (`provider`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== upstreams 增加 account_id ====================

ALTER TABLE `upstreams` ADD COLUMN `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '关联 provider_accounts.id' AFTER `model_id`;
ALTER TABLE `upstreams` ADD KEY `idx_account_id` (`account_id`);

-- ==================== 套餐定义 ====================

CREATE TABLE IF NOT EXISTS `packages` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`            VARCHAR(100) NOT NULL COMMENT '套餐名称',
    `description`     VARCHAR(500) NOT NULL DEFAULT '',
    `price_micro_usd` BIGINT       NOT NULL DEFAULT 0 COMMENT '套餐价格 (microUSD)',
    `duration_days`   INT          NOT NULL DEFAULT 0 COMMENT '有效天数, 0=永久',
    `is_active`       TINYINT      NOT NULL DEFAULT 1,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 套餐-模型关联 ====================

CREATE TABLE IF NOT EXISTS `package_models` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `package_id`      BIGINT UNSIGNED NOT NULL,
    `model_id`        BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_pkg_model` (`package_id`, `model_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 用户已购套餐 ====================

CREATE TABLE IF NOT EXISTS `user_packages` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `package_id`      BIGINT UNSIGNED NOT NULL,
    `started_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `expires_at`      DATETIME     NULL COMMENT 'NULL=永久有效',
    `status`          VARCHAR(20)  NOT NULL DEFAULT 'active' COMMENT 'active | expired | cancelled',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_package_id` (`package_id`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== api_keys 增加 key_type ====================

ALTER TABLE `api_keys` ADD COLUMN `key_type` VARCHAR(30) NOT NULL DEFAULT 'standard' COMMENT 'standard | openclaw_token' AFTER `token_group`;

-- ==================== 初始套餐 ====================

INSERT INTO `packages` (`name`, `description`, `is_active`) VALUES
('default', '默认套餐，包含基础模型', 1);
