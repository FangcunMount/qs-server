ALTER TABLE `evaluation_outcome`
  ADD COLUMN `algorithm_family` varchar(50) DEFAULT NULL AFTER `model_title`;
