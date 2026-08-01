-- 回滚 Evaluation consistency audit 的 committed outbox 证据查询联合索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'domain_event_outbox'
      AND index_name = 'idx_outbox_aggregate_event_latest'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `domain_event_outbox` DROP INDEX `idx_outbox_aggregate_event_latest`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
