DROP INDEX `idx_usage_logs_auto_policy_time` ON `usage_logs`;
DROP INDEX `idx_usage_logs_resolved_model` ON `usage_logs`;
DROP INDEX `idx_usage_logs_requested_model` ON `usage_logs`;

ALTER TABLE `usage_logs`
    DROP COLUMN `route_score`,
    DROP COLUMN `route_reason`,
    DROP COLUMN `route_mode`,
    DROP COLUMN `auto_route_policy_id`,
    DROP COLUMN `resolved_model`,
    DROP COLUMN `requested_model`;

DROP TABLE IF EXISTS `auto_route_candidates`;
DROP TABLE IF EXISTS `auto_route_policies`;

DROP INDEX `idx_models_virtual` ON `models`;

ALTER TABLE `models`
    DROP COLUMN `capability_tags`,
    DROP COLUMN `context_window_tokens`,
    DROP COLUMN `virtual_type`,
    DROP COLUMN `is_virtual`;
