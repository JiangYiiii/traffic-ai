-- 模型连通性测试结果（管理端「测试」按钮），供列表展示
ALTER TABLE `models`
  ADD COLUMN `last_test_ok` TINYINT NULL COMMENT 'NULL=未测,1=成功,0=失败' AFTER `is_active`,
  ADD COLUMN `last_test_at` DATETIME NULL AFTER `last_test_ok`,
  ADD COLUMN `last_test_latency_ms` INT NULL AFTER `last_test_at`,
  ADD COLUMN `last_test_error` VARCHAR(500) NULL AFTER `last_test_latency_ms`;
