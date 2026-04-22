-- @ai_doc 核心表结构: 用户/API Key/模型/上游线路/路由分组/余额/计费/兑换码/审计日志
-- 所有金额字段单位: 微美元 (1 USD = 1,000,000 microUSD)，整数存储避免浮点精度问题

-- ==================== 用户与认证 ====================

CREATE TABLE IF NOT EXISTS `users` (
    `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `email`          VARCHAR(255) NOT NULL,
    `password_hash`  VARCHAR(255) NOT NULL,
    `role`           VARCHAR(20)  NOT NULL DEFAULT 'default' COMMENT 'default | admin | super_admin',
    `status`         TINYINT      NOT NULL DEFAULT 1 COMMENT '1=active, 0=frozen',
    `created_at`     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_email` (`email`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== API Key (sub-token) ====================

CREATE TABLE IF NOT EXISTS `api_keys` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `name`            VARCHAR(100)    NOT NULL DEFAULT '',
    `key_hash`        VARCHAR(64)     NOT NULL COMMENT 'SHA-256 hash of raw key',
    `key_prefix`      VARCHAR(12)     NOT NULL COMMENT 'sk-xxxx prefix for display',
    `token_group`     VARCHAR(50)     NOT NULL DEFAULT 'default',
    `is_active`       TINYINT         NOT NULL DEFAULT 1,
    `expires_at`      DATETIME        NULL COMMENT 'NULL=never expires',
    `last_used_at`    DATETIME        NULL,
    `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_key_hash` (`key_hash`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_token_group` (`token_group`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 模型管理 ====================

CREATE TABLE IF NOT EXISTS `models` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `model_name`      VARCHAR(100) NOT NULL COMMENT '对外暴露的模型名，如 gpt-4o',
    `provider`        VARCHAR(50)  NOT NULL COMMENT 'openai | anthropic | google | ...',
    `model_type`      VARCHAR(30)  NOT NULL DEFAULT 'chat' COMMENT 'chat | embedding | speech | image',
    `billing_type`    VARCHAR(20)  NOT NULL DEFAULT 'per_token' COMMENT 'per_token | per_request',
    `input_price`     BIGINT       NOT NULL DEFAULT 0 COMMENT 'microUSD per 1M input tokens',
    `output_price`    BIGINT       NOT NULL DEFAULT 0 COMMENT 'microUSD per 1M output tokens',
    `reasoning_price` BIGINT       NOT NULL DEFAULT 0 COMMENT 'microUSD per 1M reasoning tokens (o1/o3)',
    `per_request_price` BIGINT     NOT NULL DEFAULT 0 COMMENT 'microUSD per request (for per_request billing)',
    `is_active`       TINYINT      NOT NULL DEFAULT 1,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_model_name` (`model_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 上游线路 ====================

CREATE TABLE IF NOT EXISTS `upstreams` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `model_id`        BIGINT UNSIGNED NOT NULL,
    `name`            VARCHAR(100) NOT NULL DEFAULT '',
    `endpoint`        VARCHAR(500) NOT NULL COMMENT '上游 API 地址',
    `credential`      TEXT         NOT NULL COMMENT 'AES-256 加密的上游凭证 (API Key/OAuth token)',
    `protocol`        VARCHAR(20)  NOT NULL DEFAULT 'chat' COMMENT 'chat | responses | messages | gemini | embeddings | speech',
    `weight`          INT          NOT NULL DEFAULT 1 COMMENT '路由权重',
    `is_active`       TINYINT      NOT NULL DEFAULT 1,
    `timeout_sec`     INT          NOT NULL DEFAULT 60,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_model_id` (`model_id`),
    KEY `idx_protocol` (`protocol`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== Token Group 路由分组 ====================

CREATE TABLE IF NOT EXISTS `token_groups` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`            VARCHAR(50)  NOT NULL COMMENT '分组名称如 default, premium',
    `description`     VARCHAR(255) NOT NULL DEFAULT '',
    `is_active`       TINYINT      NOT NULL DEFAULT 1,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Token Group 与上游线路的关联表
CREATE TABLE IF NOT EXISTS `token_group_upstreams` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `token_group_id`  BIGINT UNSIGNED NOT NULL,
    `upstream_id`     BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_group_upstream` (`token_group_id`, `upstream_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 余额管理 ====================

CREATE TABLE IF NOT EXISTS `user_balances` (
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `balance`         BIGINT       NOT NULL DEFAULT 0 COMMENT '当前余额 (microUSD)',
    `total_charged`   BIGINT       NOT NULL DEFAULT 0 COMMENT '累计充值 (microUSD)',
    `total_consumed`  BIGINT       NOT NULL DEFAULT 0 COMMENT '累计消耗 (microUSD)',
    `alert_enabled`   TINYINT      NOT NULL DEFAULT 0,
    `alert_threshold` BIGINT       NOT NULL DEFAULT 0 COMMENT '余额提醒阈值 (microUSD)',
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 余额流水 ====================

CREATE TABLE IF NOT EXISTS `balance_logs` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `amount`          BIGINT       NOT NULL COMMENT '变动金额 (microUSD)，正=充值,负=扣费',
    `balance_before`  BIGINT       NOT NULL,
    `balance_after`   BIGINT       NOT NULL,
    `reason_type`     VARCHAR(30)  NOT NULL COMMENT 'charge | consume | redeem | adjust | refund',
    `reason_detail`   VARCHAR(500) NOT NULL DEFAULT '',
    `request_id`      VARCHAR(64)  NOT NULL DEFAULT '',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_id_time` (`user_id`, `created_at`),
    KEY `idx_reason_type` (`reason_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 兑换码 ====================

CREATE TABLE IF NOT EXISTS `redeem_codes` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `code`            VARCHAR(32)  NOT NULL,
    `amount`          BIGINT       NOT NULL COMMENT '面值 (microUSD)',
    `status`          TINYINT      NOT NULL DEFAULT 0 COMMENT '0=unused, 1=used',
    `used_by`         BIGINT UNSIGNED NULL,
    `used_at`         DATETIME     NULL,
    `created_by`      BIGINT UNSIGNED NOT NULL COMMENT '创建管理员ID',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_code` (`code`),
    KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 调用日志 ====================

CREATE TABLE IF NOT EXISTS `usage_logs` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `request_id`      VARCHAR(64)  NOT NULL,
    `user_id`         BIGINT UNSIGNED NOT NULL,
    `api_key_id`      BIGINT UNSIGNED NOT NULL,
    `model`           VARCHAR(100) NOT NULL,
    `upstream_id`     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `protocol`        VARCHAR(20)  NOT NULL DEFAULT '',
    `is_stream`       TINYINT      NOT NULL DEFAULT 0,
    `status`          VARCHAR(20)  NOT NULL DEFAULT 'success' COMMENT 'success | error | timeout | rate_limited',
    `error_message`   TEXT         NOT NULL COMMENT '上游错误原文（含 JSON body）；应用层已限长，最大约 16KB',
    `input_tokens`    INT          NOT NULL DEFAULT 0,
    `output_tokens`   INT          NOT NULL DEFAULT 0,
    `reasoning_tokens` INT         NOT NULL DEFAULT 0,
    `total_tokens`    INT          NOT NULL DEFAULT 0,
    `cost_micro_usd`  BIGINT       NOT NULL DEFAULT 0,
    `latency_ms`      INT          NOT NULL DEFAULT 0,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_request_id` (`request_id`),
    KEY `idx_user_time` (`user_id`, `created_at`),
    KEY `idx_apikey_time` (`api_key_id`, `created_at`),
    KEY `idx_model` (`model`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 审计日志 ====================

CREATE TABLE IF NOT EXISTS `audit_logs` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `operator_id`     BIGINT UNSIGNED NOT NULL,
    `action`          VARCHAR(50)  NOT NULL COMMENT '操作类型',
    `target_type`     VARCHAR(50)  NOT NULL DEFAULT '' COMMENT '操作对象类型',
    `target_id`       VARCHAR(50)  NOT NULL DEFAULT '' COMMENT '操作对象ID',
    `before_data`     JSON         NULL COMMENT '变更前快照',
    `after_data`      JSON         NULL COMMENT '变更后快照',
    `ip`              VARCHAR(50)  NOT NULL DEFAULT '',
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_operator_time` (`operator_id`, `created_at`),
    KEY `idx_action` (`action`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 限流规则 ====================

CREATE TABLE IF NOT EXISTS `rate_limit_rules` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`            VARCHAR(100) NOT NULL,
    `scope`           VARCHAR(20)  NOT NULL COMMENT 'global | user | api_key | model',
    `scope_value`     VARCHAR(100) NOT NULL DEFAULT '' COMMENT '作用域值，空=全局',
    `max_rpm`         INT          NOT NULL DEFAULT 0 COMMENT '每分钟最大请求数, 0=不限',
    `max_tpm`         INT          NOT NULL DEFAULT 0 COMMENT '每分钟最大token数, 0=不限',
    `max_concurrent`  INT          NOT NULL DEFAULT 0 COMMENT '最大并发数, 0=不限',
    `is_active`       TINYINT      NOT NULL DEFAULT 1,
    `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_scope` (`scope`, `scope_value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ==================== 初始数据 ====================

INSERT INTO `token_groups` (`name`, `description`) VALUES
('default', '默认分组，所有新用户默认分配');
