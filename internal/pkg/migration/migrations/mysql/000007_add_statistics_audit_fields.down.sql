ALTER TABLE `statistics_daily`
    DROP COLUMN `created_at`,
    DROP COLUMN `updated_at`,
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;

ALTER TABLE `statistics_accumulated`
    DROP COLUMN `created_at`,
    DROP COLUMN `updated_at`,
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;

ALTER TABLE `statistics_plan`
    DROP COLUMN `created_at`,
    DROP COLUMN `updated_at`,
    DROP COLUMN `deleted_at`,
    DROP COLUMN `created_by`,
    DROP COLUMN `updated_by`,
    DROP COLUMN `deleted_by`,
    DROP COLUMN `version`;
