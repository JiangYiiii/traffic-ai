-- 回滚模型上架状态字段
-- 2026-04-16: 移除is_listed字段

-- 删除索引
DROP INDEX IF EXISTS idx_models_listed_active ON models;

-- 删除字段
ALTER TABLE models DROP COLUMN IF EXISTS is_listed;