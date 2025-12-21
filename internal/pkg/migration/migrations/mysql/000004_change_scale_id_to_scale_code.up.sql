-- 将 assessment_plan 和 assessment_task 表的 scale_id 字段改为 scale_code
-- 原因：统一使用 scale code（如 "3adyDE"）替代数字 ID，提高可读性和兼容性
-- 创建时间: 2025-12-21
-- 版本: v1
--
-- ⚠️ 重要提示：
-- 1. 如果有现有数据，需要先执行数据迁移（见 000004_change_scale_id_to_scale_code_data_migration.sql）
-- 2. 数据迁移需要从 MongoDB 的 scale 表查询 scale_id 到 scale_code 的映射
-- 3. 建议在生产环境执行前先备份数据
-- 4. 执行顺序：
--    a) 备份数据库
--    b) 执行数据迁移脚本（将 scale_id 转换为 scale_code）
--    c) 执行此结构迁移脚本（删除 scale_id，保留 scale_code）

-- ==================== assessment_plan 表 ====================

-- 1. 添加新的 scale_code 字段（先添加，用于数据迁移）
ALTER TABLE `assessment_plan` 
ADD COLUMN `scale_code` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '量表编码（如 "3adyDE"）' 
AFTER `org_id`;

-- 2. 创建新的索引
ALTER TABLE `assessment_plan` ADD INDEX `idx_scale_code` (`scale_code`);

-- 3. 数据迁移：将 scale_id 转换为 scale_code
-- 注意：此步骤需要手动执行，因为需要从 MongoDB 查询 scale 表
-- 参考 000004_change_scale_id_to_scale_code_data_migration.sql 中的说明
-- 如果已有数据迁移完成，可以跳过此步骤

-- 4. 删除旧的索引
ALTER TABLE `assessment_plan` DROP INDEX `idx_scale_id`;

-- 5. 删除旧的 scale_id 字段
ALTER TABLE `assessment_plan` DROP COLUMN `scale_id`;

-- ==================== assessment_task 表 ====================

-- 1. 添加新的 scale_code 字段（先添加，用于数据迁移）
ALTER TABLE `assessment_task` 
ADD COLUMN `scale_code` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '量表编码（冗余，用于查询优化）' 
AFTER `testee_id`;

-- 2. 创建新的索引
ALTER TABLE `assessment_task` ADD INDEX `idx_scale_code` (`scale_code`);

-- 3. 数据迁移：将 scale_id 转换为 scale_code
-- 注意：此步骤需要手动执行，因为需要从 MongoDB 查询 scale 表
-- 参考 000004_change_scale_id_to_scale_code_data_migration.sql 中的说明
-- 如果已有数据迁移完成，可以跳过此步骤

-- 4. 删除旧的索引
ALTER TABLE `assessment_task` DROP INDEX `idx_scale_id`;

-- 5. 删除旧的 scale_id 字段
ALTER TABLE `assessment_task` DROP COLUMN `scale_id`;

-- ==================== 验证 ====================
-- 执行后验证数据完整性：
-- SELECT COUNT(*) as total_plans, 
--        COUNT(CASE WHEN scale_code = '' THEN 1 END) as empty_codes
-- FROM assessment_plan 
-- WHERE deleted_at IS NULL;
--
-- SELECT COUNT(*) as total_tasks, 
--        COUNT(CASE WHEN scale_code = '' THEN 1 END) as empty_codes
-- FROM assessment_task 
-- WHERE deleted_at IS NULL;
--
-- 如果 empty_codes > 0，说明有数据未迁移成功，需要手动处理
