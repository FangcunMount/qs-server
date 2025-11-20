-- 初始化 actor 模块表结构
-- 创建时间: 2025-11-20
-- 版本: v1

-- 受试者表
CREATE TABLE IF NOT EXISTS `testee` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `org_id` bigint NOT NULL COMMENT '机构ID',
  `iam_user_id` bigint DEFAULT NULL COMMENT 'IAM用户ID（成年人）',
  `iam_child_id` bigint DEFAULT NULL COMMENT 'IAM儿童ID（未成年人）',
  `name` varchar(100) NOT NULL COMMENT '姓名',
  `gender` varchar(20) DEFAULT NULL COMMENT '性别',
  `birthday` date DEFAULT NULL COMMENT '出生日期',
  `tags` json DEFAULT NULL COMMENT '标签（JSON数组）',
  `source` varchar(50) DEFAULT NULL COMMENT '来源',
  `is_key_focus` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否重点关注',
  `total_assessments` int NOT NULL DEFAULT '0' COMMENT '总测评次数',
  `last_assessment_at` datetime DEFAULT NULL COMMENT '最后测评时间',
  `last_risk_level` varchar(50) DEFAULT NULL COMMENT '最后风险等级',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间（软删除）',
  `created_by` bigint DEFAULT NULL COMMENT '创建人ID',
  `updated_by` bigint DEFAULT NULL COMMENT '更新人ID',
  `deleted_by` bigint DEFAULT NULL COMMENT '删除人ID',
  `version` int NOT NULL DEFAULT '1' COMMENT '版本号（乐观锁）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_org_iam_user` (`org_id`,`iam_user_id`,`deleted_at`),
  UNIQUE KEY `uk_org_iam_child` (`org_id`,`iam_child_id`,`deleted_at`),
  KEY `idx_org_name` (`org_id`,`name`),
  KEY `idx_org_key_focus` (`org_id`,`is_key_focus`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='受试者表';

-- 员工表
CREATE TABLE IF NOT EXISTS `staff` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `org_id` bigint NOT NULL COMMENT '机构ID',
  `iam_user_id` bigint NOT NULL COMMENT 'IAM用户ID',
  `roles` json NOT NULL COMMENT '角色列表（JSON数组）',
  `name` varchar(100) NOT NULL COMMENT '姓名',
  `email` varchar(255) DEFAULT NULL COMMENT '邮箱',
  `phone` varchar(20) DEFAULT NULL COMMENT '电话',
  `is_active` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否激活',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `deleted_at` datetime DEFAULT NULL COMMENT '删除时间（软删除）',
  `created_by` bigint DEFAULT NULL COMMENT '创建人ID',
  `updated_by` bigint DEFAULT NULL COMMENT '更新人ID',
  `deleted_by` bigint DEFAULT NULL COMMENT '删除人ID',
  `version` int NOT NULL DEFAULT '1' COMMENT '版本号（乐观锁）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_org_iam_user` (`org_id`,`iam_user_id`,`deleted_at`),
  KEY `idx_org_active` (`org_id`,`is_active`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='员工表';
