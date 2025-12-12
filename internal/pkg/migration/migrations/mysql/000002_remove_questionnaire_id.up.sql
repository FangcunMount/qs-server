-- 删除 assessment 表中的 questionnaire_id 字段
-- 原因：已完全迁移到使用 questionnaire_code，questionnaire_id 不再需要
-- 创建时间: 2025-12-12
-- 版本: v1

-- 1. 删除与 questionnaire_id 相关的索引
ALTER TABLE `assessment` DROP INDEX `idx_questionnaire_id`;

-- 2. 删除 questionnaire_id 字段
ALTER TABLE `assessment` DROP COLUMN `questionnaire_id`;
