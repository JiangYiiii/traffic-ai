ALTER TABLE usage_logs DROP COLUMN account_id;
DROP INDEX idx_usage_logs_account_id ON usage_logs;
DROP INDEX idx_usage_logs_model_created ON usage_logs;
DROP INDEX idx_usage_logs_account_created ON usage_logs;
