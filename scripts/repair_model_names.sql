-- 一次性修复 models.model_name 常见异常（首尾空白、末尾多余句点如 gpt-5.）。
-- 用法: mysql -u root -p traffic_ai < scripts/repair_model_names.sql
-- 若 uk_model_name 冲突，请先处理重复或手工改名。

SET NAMES utf8mb4;

UPDATE models SET model_name = TRIM(model_name);

UPDATE models
SET model_name = LEFT(model_name, CHAR_LENGTH(model_name) - 1)
WHERE CHAR_LENGTH(model_name) > 1
  AND RIGHT(model_name, 1) = '.';

-- 已知错误写法（可按需增删）
UPDATE models SET model_name = 'gpt-5' WHERE model_name IN ('gpt-5.', 'gpt-5 ');

SELECT id, model_name, provider FROM models ORDER BY id;
