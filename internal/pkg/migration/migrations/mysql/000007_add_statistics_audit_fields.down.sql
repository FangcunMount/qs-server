-- created_at and updated_at belong to 000005 and intentionally survive this
-- rollback. Only fields introduced by 000007 are removed.
ALTER TABLE `statistics_daily`
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;

ALTER TABLE `statistics_accumulated`
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;

ALTER TABLE `statistics_plan`
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;
