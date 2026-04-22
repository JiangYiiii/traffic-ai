-- 移除套餐体系相关表
-- 2026-04-16: 移除packages/package_models/user_packages表，实现用户无套餐依赖的模型使用

-- 删除外键依赖（如果有的话）先删除依赖表
DROP TABLE IF EXISTS user_packages;
DROP TABLE IF EXISTS package_models;
DROP TABLE IF EXISTS packages;