ALTER TABLE usage_logs
    DROP COLUMN reasoning_effort,
    DROP COLUMN cache_read_tokens,
    DROP COLUMN cache_creation_tokens,
    DROP COLUMN note,
    DROP COLUMN client_ip;
