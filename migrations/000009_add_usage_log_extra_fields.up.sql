-- 为 usage_logs 表补齐用户控制台调用日志所需的字段：
-- 1) client_ip       — 请求方 IP，便于排查
-- 2) note            — 调用备注（失败摘要 / 其他说明）
-- 3) cache_creation_tokens / cache_read_tokens — Anthropic / Gemini 的缓存命中指标
-- 4) reasoning_effort — 推理强度（OpenAI o-series 用，后续协议可复用）
ALTER TABLE usage_logs
    ADD COLUMN client_ip              VARCHAR(64)  NOT NULL DEFAULT '' AFTER latency_ms,
    ADD COLUMN note                   VARCHAR(500) NOT NULL DEFAULT '' AFTER client_ip,
    ADD COLUMN cache_creation_tokens  INT          NOT NULL DEFAULT 0  AFTER reasoning_tokens,
    ADD COLUMN cache_read_tokens      INT          NOT NULL DEFAULT 0  AFTER cache_creation_tokens,
    ADD COLUMN reasoning_effort       VARCHAR(20)  NOT NULL DEFAULT '' AFTER cache_read_tokens;
