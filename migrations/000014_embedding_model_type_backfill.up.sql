-- 将名称明显为向量/Embedding 的模型从默认 chat 订正为 embedding，
-- 与控制台连通性探测（走 /embeddings）及 OpenClaw memorySearch 语义一致。
-- 幂等：仅影响仍为 chat 且名称匹配的存量行。

UPDATE `models`
SET `model_type` = 'embedding'
WHERE `model_type` = 'chat'
  AND (
    LOWER(`model_name`) LIKE '%embed%'
    OR LOWER(`model_name`) LIKE '%bge%'
    OR LOWER(`model_name`) LIKE 'text-embedding%'
  );
