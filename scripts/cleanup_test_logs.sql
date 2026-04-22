-- 清理测试阶段的假/脏日志，便于重新自测。
--
-- 使用场景：在开发库执行；生产库禁止运行。
-- 副作用：
--   * 调用日志 usage_logs 全量清空（AUTO_INCREMENT 会重置）；
--   * 余额流水中仅清理与网关调用相关的 consume/refund（保留 charge / redeem 等人工/兑换操作）；
--   * 余额 user_balances.total_consumed 回零，避免与已清空的流水对不上。
--
-- 执行前请再次确认目标库名；示例：
--   mysql -uroot -p myf_loan_dev < scripts/cleanup_test_logs.sql
SET FOREIGN_KEY_CHECKS = 0;

TRUNCATE TABLE usage_logs;

DELETE FROM balance_logs WHERE reason_type IN ('consume', 'refund');

UPDATE user_balances SET total_consumed = 0;

SET FOREIGN_KEY_CHECKS = 1;
