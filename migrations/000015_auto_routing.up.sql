-- Global AUTO routing:
-- 1) models can represent virtual AUTO entries and real-model capabilities.
-- 2) auto_route_policies / auto_route_candidates store administrator-managed global AUTO routes.
-- 3) usage_logs records requested vs resolved model for explainable billing and routing.

ALTER TABLE `models`
    ADD COLUMN `is_virtual` TINYINT NOT NULL DEFAULT 0 AFTER `is_listed`,
    ADD COLUMN `virtual_type` VARCHAR(30) NOT NULL DEFAULT '' AFTER `is_virtual`,
    ADD COLUMN `context_window_tokens` INT NOT NULL DEFAULT 0 AFTER `virtual_type`,
    ADD COLUMN `capability_tags` JSON NULL AFTER `context_window_tokens`;

CREATE INDEX `idx_models_virtual` ON `models`(`is_virtual`, `virtual_type`);

CREATE TABLE IF NOT EXISTS `auto_route_policies` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `virtual_model_id` BIGINT UNSIGNED NOT NULL,
    `name` VARCHAR(100) NOT NULL DEFAULT '',
    `strategy` VARCHAR(30) NOT NULL DEFAULT 'balanced',
    `rules_json` JSON NULL,
    `is_active` TINYINT NOT NULL DEFAULT 1,
    `version` INT NOT NULL DEFAULT 1,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_virtual_model` (`virtual_model_id`),
    KEY `idx_active_strategy` (`is_active`, `strategy`),
    CONSTRAINT `fk_auto_route_policy_virtual_model`
        FOREIGN KEY (`virtual_model_id`) REFERENCES `models`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `auto_route_candidates` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `policy_id` BIGINT UNSIGNED NOT NULL,
    `target_model_id` BIGINT UNSIGNED NOT NULL,
    `priority` INT NOT NULL DEFAULT 100,
    `weight` INT NOT NULL DEFAULT 1,
    `min_request_context_tokens` INT NOT NULL DEFAULT 0,
    `quality_score` INT NOT NULL DEFAULT 50,
    `cost_bias` INT NOT NULL DEFAULT 0,
    `latency_bias` INT NOT NULL DEFAULT 0,
    `is_active` TINYINT NOT NULL DEFAULT 1,
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_policy_target_model` (`policy_id`, `target_model_id`),
    KEY `idx_policy_active` (`policy_id`, `is_active`),
    CONSTRAINT `fk_auto_route_candidate_policy`
        FOREIGN KEY (`policy_id`) REFERENCES `auto_route_policies`(`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_auto_route_candidate_target_model`
        FOREIGN KEY (`target_model_id`) REFERENCES `models`(`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE `usage_logs`
    ADD COLUMN `requested_model` VARCHAR(100) NOT NULL DEFAULT '' AFTER `model`,
    ADD COLUMN `resolved_model` VARCHAR(100) NOT NULL DEFAULT '' AFTER `requested_model`,
    ADD COLUMN `auto_route_policy_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `model_account_id`,
    ADD COLUMN `route_mode` VARCHAR(30) NOT NULL DEFAULT '' AFTER `auto_route_policy_id`,
    ADD COLUMN `route_reason` JSON NULL AFTER `route_mode`,
    ADD COLUMN `route_score` INT NOT NULL DEFAULT 0 AFTER `route_reason`;

UPDATE `usage_logs`
SET `requested_model` = `model`, `resolved_model` = `model`
WHERE `requested_model` = '' AND `resolved_model` = '';

CREATE INDEX `idx_usage_logs_requested_model` ON `usage_logs`(`requested_model`);
CREATE INDEX `idx_usage_logs_resolved_model` ON `usage_logs`(`resolved_model`);
CREATE INDEX `idx_usage_logs_auto_policy_time` ON `usage_logs`(`auto_route_policy_id`, `created_at`);
