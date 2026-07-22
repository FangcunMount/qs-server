-- Historical replay compatibility note:
-- 000005 already creates created_at/updated_at in a fresh database, while some
-- deployed schemas reached this migration without all audit columns. Keep the
-- original version number, but add only columns that are actually absent so the
-- chain is replayable without changing already-migrated databases.

SET @columns = (
  SELECT GROUP_CONCAT(spec.ddl ORDER BY spec.ord SEPARATOR ', ')
  FROM (
    SELECT 1 ord, 'created_at' name, 'ADD COLUMN `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP' ddl UNION ALL
    SELECT 2, 'updated_at', 'ADD COLUMN `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP' UNION ALL
    SELECT 3, 'deleted_at', 'ADD COLUMN `deleted_at` DATETIME NULL' UNION ALL
    SELECT 4, 'created_by', 'ADD COLUMN `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 5, 'updated_by', 'ADD COLUMN `updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 6, 'deleted_by', 'ADD COLUMN `deleted_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 7, 'version', 'ADD COLUMN `version` INT UNSIGNED NOT NULL DEFAULT 1'
  ) spec
  LEFT JOIN information_schema.columns col
    ON col.table_schema = DATABASE() AND col.table_name = 'statistics_daily' AND col.column_name = spec.name
  WHERE col.column_name IS NULL
);
SET @ddl = IF(@columns IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `statistics_daily` ', @columns));
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @columns = (
  SELECT GROUP_CONCAT(spec.ddl ORDER BY spec.ord SEPARATOR ', ')
  FROM (
    SELECT 1 ord, 'created_at' name, 'ADD COLUMN `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP' ddl UNION ALL
    SELECT 2, 'updated_at', 'ADD COLUMN `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP' UNION ALL
    SELECT 3, 'deleted_at', 'ADD COLUMN `deleted_at` DATETIME NULL' UNION ALL
    SELECT 4, 'created_by', 'ADD COLUMN `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 5, 'updated_by', 'ADD COLUMN `updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 6, 'deleted_by', 'ADD COLUMN `deleted_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 7, 'version', 'ADD COLUMN `version` INT UNSIGNED NOT NULL DEFAULT 1'
  ) spec
  LEFT JOIN information_schema.columns col
    ON col.table_schema = DATABASE() AND col.table_name = 'statistics_accumulated' AND col.column_name = spec.name
  WHERE col.column_name IS NULL
);
SET @ddl = IF(@columns IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `statistics_accumulated` ', @columns));
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @columns = (
  SELECT GROUP_CONCAT(spec.ddl ORDER BY spec.ord SEPARATOR ', ')
  FROM (
    SELECT 1 ord, 'created_at' name, 'ADD COLUMN `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP' ddl UNION ALL
    SELECT 2, 'updated_at', 'ADD COLUMN `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP' UNION ALL
    SELECT 3, 'deleted_at', 'ADD COLUMN `deleted_at` DATETIME NULL' UNION ALL
    SELECT 4, 'created_by', 'ADD COLUMN `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 5, 'updated_by', 'ADD COLUMN `updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 6, 'deleted_by', 'ADD COLUMN `deleted_by` BIGINT UNSIGNED NOT NULL DEFAULT 0' UNION ALL
    SELECT 7, 'version', 'ADD COLUMN `version` INT UNSIGNED NOT NULL DEFAULT 1'
  ) spec
  LEFT JOIN information_schema.columns col
    ON col.table_schema = DATABASE() AND col.table_name = 'statistics_plan' AND col.column_name = spec.name
  WHERE col.column_name IS NULL
);
SET @ddl = IF(@columns IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `statistics_plan` ', @columns));
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
