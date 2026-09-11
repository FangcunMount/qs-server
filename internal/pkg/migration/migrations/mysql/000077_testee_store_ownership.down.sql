-- 执行前必须备份归属与历史；已有归属的生产环境优先保留新增结构并向前修复。
DROP TABLE testee_store_history;
ALTER TABLE testee DROP INDEX idx_testee_org_store, DROP COLUMN store_version, DROP COLUMN store_id;
