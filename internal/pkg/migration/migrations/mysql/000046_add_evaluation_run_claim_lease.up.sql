ALTER TABLE `runtime_checkpoint`
  ADD COLUMN `claim_token` varchar(100) DEFAULT NULL COMMENT 'evaluation run claim fencing token' AFTER `input_snapshot_ref`,
  ADD COLUMN `lease_expires_at` datetime(3) DEFAULT NULL COMMENT 'evaluation run claim lease expiry' AFTER `claim_token`,
  ADD KEY `idx_runtime_checkpoint_claim` (`scope`, `status`, `lease_expires_at`);
