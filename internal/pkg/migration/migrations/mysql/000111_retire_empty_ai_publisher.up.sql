-- Only the exact bound migration driver may set this session guard while
-- holding the table WRITE lock and proving COUNT(*)=0. Direct SQL use fails.
SET @qs_m5_ai_publisher_sql = IF(COALESCE(@qs_m5_empty_ai_publisher_guard, 0)=1,
  'DROP TABLE `ai_messaging_outbox`',
  'SELECT qs_m5_ai_publisher_requires_bound_empty_guard()');
PREPARE qs_m5_ai_publisher_stmt FROM @qs_m5_ai_publisher_sql;
EXECUTE qs_m5_ai_publisher_stmt;
-- The driver always releases the statement, guard, table lock and wait setting.
