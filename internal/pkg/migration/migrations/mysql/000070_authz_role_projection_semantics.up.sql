ALTER TABLE `staff`
    ADD COLUMN `effective_roles` JSON NULL AFTER `roles`,
    ADD COLUMN `authz_policy_version` BIGINT NOT NULL DEFAULT 0 AFTER `effective_roles`,
    ADD COLUMN `authz_projected_at` DATETIME(3) NULL AFTER `authz_policy_version`,
    ADD COLUMN `authz_projection_pending` BOOLEAN NOT NULL DEFAULT FALSE AFTER `authz_projected_at`;

UPDATE `staff`
SET `effective_roles` = `roles`
WHERE `effective_roles` IS NULL;

ALTER TABLE `staff`
    MODIFY COLUMN `effective_roles` JSON NOT NULL;
