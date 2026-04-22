-- 回退 000012：TEXT → VARCHAR(500)
-- 注意：回退前先把现有超长记录截断到 500 字节，否则 MODIFY COLUMN 会因数据越界失败。

UPDATE `usage_logs`
SET `error_message` = LEFT(`error_message`, 500)
WHERE CHAR_LENGTH(`error_message`) > 500;

ALTER TABLE `usage_logs`
    MODIFY COLUMN `error_message` VARCHAR(500) NOT NULL DEFAULT '';
