-- 回滚：恢复 assessment 表中的 questionnaire_id 字段
-- 注意：此操作会导致数据丢失，因为 questionnaire_id 已不再使用
-- 创建时间: 2025-12-12
-- 版本: v1

-- 1. 添加 questionnaire_id 字段（设置默认值0以允许现有数据）
ALTER TABLE `assessment` 
ADD COLUMN `questionnaire_id` bigint unsigned NOT NULL DEFAULT 0 COMMENT '问卷ID（已废弃）' 
AFTER `testee_id`;

-- 2. 创建索引
ALTER TABLE `assessment` ADD INDEX `idx_questionnaire_id` (`questionnaire_id`);

-- 警告：此回滚操作无法恢复原有的 questionnaire_id 数据
-- 所有现有记录的 questionnaire_id 将被设置为 0
