-- 回滚套餐体系表（从原有迁移文件重建）
-- 2026-04-16: 恢复packages相关表结构

CREATE TABLE IF NOT EXISTS `packages` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`            VARCHAR(100)    NOT NULL COMMENT '套餐名称',
    `description`     VARCHAR(500)    NOT NULL DEFAULT '' COMMENT '套餐描述',
    `price_micro_usd` BIGINT          NOT NULL DEFAULT 0 COMMENT '套餐价格 (microUSD)',
    `duration_days`   INT             NOT NULL DEFAULT 0 COMMENT '有效天数，0=永久',
    `is_active`       TINYINT         NOT NULL DEFAULT 1 COMMENT '是否启用',
    `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='套餐定义表';

CREATE TABLE IF NOT EXISTS `package_models` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `package_id` BIGINT UNSIGNED NOT NULL,
    `model_id`   BIGINT UNSIGNED NOT NULL,
    `created_at` DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_pkg_model` (`package_id`, `model_id`),
    KEY `idx_package_id` (`package_id`),
    KEY `idx_model_id` (`model_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='套餐-模型关联表';

CREATE TABLE IF NOT EXISTS `user_packages` (
    `id`         BIGINT UNSIGNED                           NOT NULL AUTO_INCREMENT,
    `user_id`    BIGINT UNSIGNED                           NOT NULL,
    `package_id` BIGINT UNSIGNED                           NOT NULL,
    `started_at` DATETIME                                  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `expires_at` DATETIME                                  NULL COMMENT '到期时间，NULL=永久',
    `status`     ENUM('active', 'expired', 'cancelled')   NOT NULL DEFAULT 'active',
    `created_at` DATETIME                                  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME                                  NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_package_id` (`package_id`),
    KEY `idx_status` (`status`),
    KEY `idx_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户套餐购买记录';

-- 插入默认套餐
INSERT IGNORE INTO `packages` (`name`, `description`, `price_micro_usd`, `duration_days`, `is_active`) 
VALUES ('default', '默认套餐', 0, 0, 1);