-- 添加模型上架状态字段
-- 2026-04-16: 为models表添加is_listed字段，用于控制模型是否对用户可见

ALTER TABLE models ADD COLUMN is_listed TINYINT DEFAULT 0 COMMENT '是否上架展示给用户 0=未上架 1=已上架';

-- 为现有启用模型设置为上架
UPDATE models SET is_listed = 1 WHERE is_active = 1;

-- 添加索引优化查询性能
CREATE INDEX idx_models_listed_active ON models(is_listed, is_active);