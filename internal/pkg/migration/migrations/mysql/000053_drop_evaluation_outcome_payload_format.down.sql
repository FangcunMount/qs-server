ALTER TABLE `evaluation_outcome`
  ADD COLUMN `payload_format` varchar(100) DEFAULT NULL COMMENT 'retired compatibility field' AFTER `decision_kind`;
