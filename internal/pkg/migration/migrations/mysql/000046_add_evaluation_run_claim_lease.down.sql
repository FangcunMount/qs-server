ALTER TABLE `runtime_checkpoint`
  DROP KEY `idx_runtime_checkpoint_claim`,
  DROP COLUMN `lease_expires_at`,
  DROP COLUMN `claim_token`;
