-- 修复存量 endpoint：旧逻辑会剥离末尾 /v1，新逻辑保留完整版本路径，拼接时只追加 /chat/completions。
-- 仅对已知 /v1 厂商且当前不含版本后缀的记录补回 /v1。

UPDATE upstreams u
JOIN models m ON u.model_id = m.id
SET u.endpoint = CONCAT(u.endpoint, '/v1')
WHERE m.provider IN ('openai', 'deepseek', 'moonshot', 'qwen', 'hunyuan', 'spark', 'baichuan', 'yi', 'minimax', 'stepfun')
  AND u.endpoint NOT LIKE '%/v1'
  AND u.endpoint NOT LIKE '%/v2'
  AND u.endpoint NOT LIKE '%/v3'
  AND u.endpoint NOT LIKE '%/v4'
  AND u.endpoint NOT LIKE '%/v1beta';
