-- ==================== 统计模块表结构 ====================
-- 创建时间：2025-01-XX
-- 说明：统计模块采用三层架构：Redis预聚合 + MySQL统计表 + 原始表聚合兜底

-- ==================== 1. 每日统计表（时间序列数据） ====================
CREATE TABLE IF NOT EXISTS `statistics_daily` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `org_id` BIGINT NOT NULL COMMENT '机构ID',
    `statistic_type` VARCHAR(50) NOT NULL COMMENT '统计类型：questionnaire/testee/plan/screening',
    `statistic_key` VARCHAR(255) NOT NULL COMMENT '统计键（如 questionnaire_code、testee_id）',
    `stat_date` DATE NOT NULL COMMENT '统计日期',
    
    -- 计数指标
    `submission_count` BIGINT NOT NULL DEFAULT 0 COMMENT '提交数',
    `completion_count` BIGINT NOT NULL DEFAULT 0 COMMENT '完成数',
    
    -- 扩展指标（JSON，支持灵活扩展）
    `extra_metrics` JSON COMMENT '扩展指标：{"risk_distribution": {...}, "origin_distribution": {...}}',
    
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY `uk_org_type_key_date` (`org_id`, `statistic_type`, `statistic_key`, `stat_date`),
    KEY `idx_org_date` (`org_id`, `stat_date`),
    KEY `idx_type_key` (`statistic_type`, `statistic_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日统计表';

-- ==================== 2. 累计统计表（维度预聚合） ====================
-- 设计理念：统一表结构，通过 statistic_type 区分不同维度
CREATE TABLE IF NOT EXISTS `statistics_accumulated` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `org_id` BIGINT NOT NULL COMMENT '机构ID',
    `statistic_type` VARCHAR(50) NOT NULL COMMENT '统计类型：questionnaire/testee/plan/screening/system',
    `statistic_key` VARCHAR(255) NOT NULL COMMENT '统计键',
    
    -- 基础指标
    `total_submissions` BIGINT NOT NULL DEFAULT 0 COMMENT '总提交数',
    `total_completions` BIGINT NOT NULL DEFAULT 0 COMMENT '总完成数',
    
    -- 时间窗口指标（从 statistics_daily 聚合）
    `last7d_submissions` BIGINT NOT NULL DEFAULT 0 COMMENT '近7天提交数',
    `last15d_submissions` BIGINT NOT NULL DEFAULT 0 COMMENT '近15天提交数',
    `last30d_submissions` BIGINT NOT NULL DEFAULT 0 COMMENT '近30天提交数',
    
    -- 分布指标（JSON）
    `distribution` JSON COMMENT '分布数据：{"risk": {...}, "origin": {...}, "status": {...}}',
    
    -- 时间维度
    `first_occurred_at` TIMESTAMP NULL COMMENT '首次发生时间',
    `last_occurred_at` TIMESTAMP NULL COMMENT '最近发生时间',
    
    -- 最后更新时间
    `last_updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY `uk_org_type_key` (`org_id`, `statistic_type`, `statistic_key`),
    KEY `idx_org_type` (`org_id`, `statistic_type`),
    KEY `idx_type_key` (`statistic_type`, `statistic_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='累计统计表';

-- ==================== 3. 计划任务统计表（计划维度专用） ====================
-- 设计理念：计划统计需要关联任务表，单独设计更清晰
CREATE TABLE IF NOT EXISTS `statistics_plan` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `org_id` BIGINT NOT NULL COMMENT '机构ID',
    `plan_id` BIGINT UNSIGNED NOT NULL COMMENT '计划ID',
    
    -- 任务统计
    `total_tasks` BIGINT NOT NULL DEFAULT 0 COMMENT '总任务数',
    `completed_tasks` BIGINT NOT NULL DEFAULT 0 COMMENT '已完成任务数',
    `pending_tasks` BIGINT NOT NULL DEFAULT 0 COMMENT '待完成任务数',
    `expired_tasks` BIGINT NOT NULL DEFAULT 0 COMMENT '已过期任务数',
    
    -- 受试者统计
    `enrolled_testees` BIGINT NOT NULL DEFAULT 0 COMMENT '已加入计划的受试者数',
    `active_testees` BIGINT NOT NULL DEFAULT 0 COMMENT '活跃受试者数（有完成任务的）',
    
    -- 最后更新时间
    `last_updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    UNIQUE KEY `uk_plan` (`org_id`, `plan_id`),
    KEY `idx_org_id` (`org_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='计划统计表';

