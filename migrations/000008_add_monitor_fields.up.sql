-- 为 usage_logs 表添加 account_id 字段，支持按账号维度监控
ALTER TABLE usage_logs ADD COLUMN account_id BIGINT NOT NULL DEFAULT 0 AFTER upstream_id;

-- 添加索引优化监控查询性能
CREATE INDEX idx_usage_logs_account_id ON usage_logs(account_id);
CREATE INDEX idx_usage_logs_model_created ON usage_logs(model, created_at);
CREATE INDEX idx_usage_logs_account_created ON usage_logs(account_id, created_at);
