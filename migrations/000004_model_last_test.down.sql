ALTER TABLE `models`
  DROP COLUMN `last_test_error`,
  DROP COLUMN `last_test_latency_ms`,
  DROP COLUMN `last_test_at`,
  DROP COLUMN `last_test_ok`;
