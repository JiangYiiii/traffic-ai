-- OAuth 授权流程临时状态表
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

-- upstreams 表扩展 OAuth 字段
ALTER TABLE `upstreams`
    ADD COLUMN `auth_type` VARCHAR(20) NOT NULL DEFAULT 'api_key' COMMENT 'api_key | oauth_authorization_code' AFTER `credential`,
    ADD COLUMN `refresh_token` TEXT COMMENT 'AES-256 加密的 refresh_token（仅 OAuth 类型）' AFTER `auth_type`,
    ADD COLUMN `token_expires_at` DATETIME DEFAULT NULL COMMENT 'access_token 过期时间' AFTER `refresh_token`;
