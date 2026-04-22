-- 回滚：将补回的 /v1 再次剥离

UPDATE upstreams u
JOIN models m ON u.model_id = m.id
SET u.endpoint = TRIM(TRAILING '/v1' FROM u.endpoint)
WHERE m.provider IN ('openai', 'deepseek', 'moonshot', 'qwen', 'hunyuan', 'spark', 'baichuan', 'yi', 'minimax', 'stepfun')
  AND u.endpoint LIKE '%/v1';
