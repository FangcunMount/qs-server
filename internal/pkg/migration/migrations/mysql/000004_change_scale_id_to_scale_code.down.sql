-- 回滚：将 assessment_plan 和 assessment_task 表的 scale_code 字段改回 scale_id
-- 注意：此操作会导致数据丢失，因为 scale_code 无法直接转换为 scale_id
-- 创建时间: 2025-12-21
-- 版本: v1
--
-- ⚠️ 警告：
-- 1. 此回滚操作无法恢复原有的 scale_id 数据
-- 2. 所有现有记录的 scale_id 将被设置为 0
-- 3. 如果需要恢复数据，需要从备份中恢复或手动查询 scale 表进行转换

-- ==================== assessment_task 表 ====================

-- 1. 添加 scale_id 字段（设置默认值0以允许现有数据）
ALTER TABLE `assessment_task` 
ADD COLUMN `scale_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '量表ID（已废弃）' 
AFTER `testee_id`;

-- 2. 创建索引
ALTER TABLE `assessment_task` ADD INDEX `idx_scale_id` (`scale_id`);

-- 3. 删除新的索引
ALTER TABLE `assessment_task` DROP INDEX `idx_scale_code`;

-- 4. 删除 scale_code 字段
ALTER TABLE `assessment_task` DROP COLUMN `scale_code`;

-- ==================== assessment_plan 表 ====================

-- 1. 添加 scale_id 字段（设置默认值0以允许现有数据）
ALTER TABLE `assessment_plan` 
ADD COLUMN `scale_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '量表ID（已废弃）' 
AFTER `org_id`;

-- 2. 创建索引
ALTER TABLE `assessment_plan` ADD INDEX `idx_scale_id` (`scale_id`);

-- 3. 删除新的索引
ALTER TABLE `assessment_plan` DROP INDEX `idx_scale_code`;

-- 4. 删除 scale_code 字段
ALTER TABLE `assessment_plan` DROP COLUMN `scale_code`;

