-- 生产环境 baseline 配置数据（幂等，可重复执行）
-- 模型、模型账号、AUTO 路由策略等业务数据请在管理后台配置，勿写入此文件。

SET NAMES utf8mb4;

-- 默认 Token 分组（000001 已插入时 IGNORE 跳过）
INSERT IGNORE INTO `token_groups` (`name`, `description`) VALUES
('default', '默认分组，所有新用户默认分配');

-- 全局并发兜底（与 configs/config.prod.yaml.example gateway 段配合；0=不限 RPM/TPM）
INSERT INTO `rate_limit_rules` (`name`, `scope`, `scope_value`, `max_rpm`, `max_tpm`, `max_concurrent`, `is_active`)
SELECT 'global-default-concurrency', 'global', '', 0, 0, 500, 1
WHERE NOT EXISTS (
  SELECT 1 FROM `rate_limit_rules`
  WHERE `scope` = 'global' AND `scope_value` = '' AND `name` = 'global-default-concurrency'
);

SELECT 'deploy/seed/001_baseline.sql: done' AS status;
