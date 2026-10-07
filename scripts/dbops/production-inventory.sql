-- Fresh connection only. No --force: any failed query rejects this inventory.
SET SESSION max_execution_time = 5000;
SET SESSION lock_wait_timeout = 5;
START TRANSACTION READ ONLY;
SELECT 'server', DATABASE(), VERSION();
-- InnoDB row/space values are metadata estimates, never proof of emptiness.
SELECT 'tables', table_name, table_type, engine, table_rows, data_length, index_length
FROM information_schema.tables WHERE table_schema = DATABASE()
ORDER BY table_name LIMIT 1001;
-- Do not project defaults, comments, enum literals or generated-expression bodies.
SELECT 'columns', table_name, ordinal_position, column_name, data_type,
       is_nullable, character_maximum_length, numeric_precision
FROM information_schema.columns WHERE table_schema = DATABASE()
ORDER BY table_name, ordinal_position LIMIT 10001;
SELECT 'indexes', table_name, index_name, non_unique, seq_in_index,
       column_name, sub_part, index_type, is_visible
FROM information_schema.statistics WHERE table_schema = DATABASE()
ORDER BY table_name, index_name, seq_in_index LIMIT 10001;
SELECT 'migration', version, dirty FROM schema_migrations LIMIT 2;
ROLLBACK;
