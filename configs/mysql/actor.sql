-- Actor 模块数据库表结构
-- Testee (受试者) 和 Staff (员工) 表

USE `questionnaire`;

-- ============================================
-- Testee (受试者) 表
-- ============================================
DROP TABLE IF EXISTS `testee`;
CREATE TABLE `testee` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '受试者ID',
  `org_id` bigint(20) NOT NULL COMMENT '所属机构ID',
  `iam_user_id` bigint(20) DEFAULT NULL COMMENT 'IAM用户ID（成人）',
  `iam_child_id` bigint(20) DEFAULT NULL COMMENT 'IAM儿童ID',
  `name` varchar(100) NOT NULL COMMENT '姓名',
  `gender` tinyint(4) NOT NULL COMMENT '性别：1-男，2-女，3-其他',
  `birthday` date DEFAULT NULL COMMENT '出生日期',
  `tags` json DEFAULT NULL COMMENT '标签列表',
  `source` varchar(50) NOT NULL DEFAULT 'unknown' COMMENT '来源：online_form/plan/screening/imported',
  `is_key_focus` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否重点关注：0-否，1-是',
  
  -- 测评统计字段
  `total_assessments` int(11) NOT NULL DEFAULT 0 COMMENT '总测评次数',
  `last_assessment_at` datetime DEFAULT NULL COMMENT '最后测评时间',
  `last_risk_level` varchar(50) DEFAULT NULL COMMENT '最后风险等级',
  
  -- 审计字段
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `deleted_at` timestamp NULL DEFAULT NULL COMMENT '删除时间（软删除）',
  `created_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '创建人ID',
  `updated_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '更新人ID',
  `deleted_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '删除人ID',
  `version` int(10) unsigned NOT NULL DEFAULT 1 COMMENT '乐观锁版本号',
  
  PRIMARY KEY (`id`),
  KEY `idx_org_id` (`org_id`),
  KEY `idx_iam_user_id` (`iam_user_id`),
  KEY `idx_iam_child_id` (`iam_child_id`),
  KEY `idx_name` (`name`),
  KEY `idx_is_key_focus` (`is_key_focus`),
  KEY `idx_deleted_at` (`deleted_at`),
  UNIQUE KEY `uk_org_iam_user` (`org_id`, `iam_user_id`, `deleted_at`),
  UNIQUE KEY `uk_org_iam_child` (`org_id`, `iam_child_id`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='受试者表';

-- ============================================
-- Staff (员工) 表
-- ============================================
DROP TABLE IF EXISTS `staff`;
CREATE TABLE `staff` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '员工ID',
  `org_id` bigint(20) NOT NULL COMMENT '所属机构ID',
  `iam_user_id` bigint(20) NOT NULL COMMENT 'IAM用户ID（必须绑定）',
  `roles` json NOT NULL COMMENT '业务角色列表',
  `name` varchar(100) NOT NULL COMMENT '姓名（冗余缓存）',
  `email` varchar(255) DEFAULT NULL COMMENT '邮箱（冗余缓存）',
  `phone` varchar(20) DEFAULT NULL COMMENT '手机号（冗余缓存）',
  `is_active` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否激活：0-停用，1-激活',
  
  -- 审计字段
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `deleted_at` timestamp NULL DEFAULT NULL COMMENT '删除时间（软删除）',
  `created_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '创建人ID',
  `updated_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '更新人ID',
  `deleted_by` bigint(20) unsigned NOT NULL DEFAULT 0 COMMENT '删除人ID',
  `version` int(10) unsigned NOT NULL DEFAULT 1 COMMENT '乐观锁版本号',
  
  PRIMARY KEY (`id`),
  KEY `idx_org_id` (`org_id`),
  KEY `idx_iam_user_id` (`iam_user_id`),
  KEY `idx_is_active` (`is_active`),
  KEY `idx_deleted_at` (`deleted_at`),
  UNIQUE KEY `uk_org_iam_user` (`org_id`, `iam_user_id`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='员工表';

-- ============================================
-- 索引说明
-- ============================================
-- testee 表：
--   - uk_org_iam_user: 保证同一机构同一IAM用户只有一个受试者记录（含软删除）
--   - uk_org_iam_child: 保证同一机构同一IAM儿童只有一个受试者记录（含软删除）
--   - idx_is_key_focus: 快速查询重点关注的受试者
--   - idx_name: 支持按姓名模糊查询
--
-- staff 表：
--   - uk_org_iam_user: 保证同一机构同一IAM用户只有一个员工记录（含软删除）
--   - idx_is_active: 快速查询激活状态的员工
