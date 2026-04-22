-- 000012: usage_logs.error_message 从 VARCHAR(500) 扩容到 TEXT
--
-- 背景：
--   Anthropic 的 rate_limit_error / 阿里云 Arrearage 等完整 JSON body 动辄 700-900 字节，
--   加上 usecase 层的 "upstream %d: %s" 前缀后超过 500，触发：
--     Error 1406 (22001): Data too long for column 'error_message' at row 1
--   导致 usage_logs 写入失败，运维侧看不到真实上游错误，完全掩盖根因。
--
-- 策略：
--   列改为 TEXT（上限 64KB），应用层仍做 16KB 截断兜底，避免极端情况把日志表撑爆。

ALTER TABLE `usage_logs`
    MODIFY COLUMN `error_message` TEXT NOT NULL
    COMMENT '上游错误原文（含 JSON body）；应用层已限长，最大约 16KB';
