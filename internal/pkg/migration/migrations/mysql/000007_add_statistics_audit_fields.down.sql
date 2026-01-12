ALTER TABLE `statistics_daily`
    DROP COLUMN IF EXISTS `created_at`,
    DROP COLUMN IF EXISTS `updated_at`,
    DROP COLUMN IF EXISTS `deleted_at`,
    DROP COLUMN IF EXISTS `created_by`,
    DROP COLUMN IF EXISTS `updated_by`,
    DROP COLUMN IF EXISTS `deleted_by`,
    DROP COLUMN IF EXISTS `version`;

ALTER TABLE `statistics_accumulated`
    DROP COLUMN IF EXISTS `created_at`,
    DROP COLUMN IF EXISTS `updated_at`,
    DROP COLUMN IF EXISTS `deleted_at`,
    DROP COLUMN IF EXISTS `created_by`,
    DROP COLUMN IF EXISTS `updated_by`,
    DROP COLUMN IF EXISTS `deleted_by`,
    DROP COLUMN IF EXISTS `version`;

ALTER TABLE `statistics_plan`
    DROP COLUMN IF EXISTS `created_at`,
    DROP COLUMN IF EXISTS `updated_at`,
    DROP COLUMN IF EXISTS `deleted_at`,
    DROP COLUMN IF EXISTS `created_by`,
    DROP COLUMN IF EXISTS `updated_by`,
    DROP COLUMN IF EXISTS `deleted_by`,
    DROP COLUMN IF EXISTS `version`;
