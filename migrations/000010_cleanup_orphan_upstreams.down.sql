-- 000010 down: 无需还原。
-- 被清理的是 model 删除后的孤儿 upstream，本就是应删除的垃圾数据；
-- 没有可靠方法还原（原 model 已经被删除，provider/endpoint/credential 等上下文都已丢失）。
--
-- 此文件存在仅为满足 golang-migrate 的命名要求。
SELECT 1;
