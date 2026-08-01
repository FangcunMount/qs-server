-- 为 Evaluation consistency audit 的 committed outbox 证据查询补联合索引。
-- 说明：
-- - aggregate_type + aggregate_id 定位业务聚合
-- - event_type 定位该聚合的 committed 事件
-- - id 支持同一聚合事件按最新记录倒序读取
-- - 非唯一索引保留一致性审计发现重复事件的能力

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'domain_event_outbox'
      AND index_name = 'idx_outbox_aggregate_event_latest'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_aggregate_event_latest` (`aggregate_type`, `aggregate_id`, `event_type`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
