UPDATE `staff`
SET `roles` = `effective_roles`
WHERE `effective_roles` IS NOT NULL;

ALTER TABLE `staff`
    DROP COLUMN `authz_projection_pending`,
    DROP COLUMN `authz_projected_at`,
    DROP COLUMN `authz_policy_version`,
    DROP COLUMN `effective_roles`;
