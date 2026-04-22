-- 单条模型账号的连通性测试结果（列表按账号展示）
ALTER TABLE `model_accounts`
  ADD COLUMN `last_test_ok` TINYINT NULL COMMENT 'NULL=未测,1=成功,0=失败' AFTER `timeout_sec`,
  ADD COLUMN `last_test_at` DATETIME NULL AFTER `last_test_ok`,
  ADD COLUMN `last_test_latency_ms` INT NULL AFTER `last_test_at`,
  ADD COLUMN `last_test_error` VARCHAR(500) NULL AFTER `last_test_latency_ms`;
