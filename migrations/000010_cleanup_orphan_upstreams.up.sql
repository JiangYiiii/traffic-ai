-- 000010: 清理孤儿 upstream
--
-- 背景：
--   历史数据里存在 model_id 指向已删除 models 的 upstream 记录（7 条）。
--   这些孤儿记录无法被任何请求路由到，也未被任何 usage_logs / token_group_upstreams 引用，
--   属于 model 删除时未级联清理 upstream 遗留的垃圾数据。
--
-- 风险：无。
--   - 已确认 usage_logs 对这些孤儿 upstream 的引用次数为 0
--   - 已确认 token_group_upstreams 对这些孤儿 upstream 的引用次数为 0
--
-- down.sql 为空：垃圾数据不需要还原。

DELETE FROM `upstreams`
WHERE `model_id` NOT IN (SELECT `id` FROM `models`);
