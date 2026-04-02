-- 清理同一计划/受试者/序号下的重复任务，保留优先级更高且 ID 更大的那一条。
-- 优先级：completed > opened > pending > expired > canceled。
DELETE t1
FROM `assessment_task` t1
JOIN `assessment_task` t2
  ON t1.`plan_id` = t2.`plan_id`
 AND t1.`testee_id` = t2.`testee_id`
 AND t1.`seq` = t2.`seq`
 AND (
      CASE t1.`status`
        WHEN 'completed' THEN 5
        WHEN 'opened' THEN 4
        WHEN 'pending' THEN 3
        WHEN 'expired' THEN 2
        WHEN 'canceled' THEN 1
        ELSE 0
      END <
      CASE t2.`status`
        WHEN 'completed' THEN 5
        WHEN 'opened' THEN 4
        WHEN 'pending' THEN 3
        WHEN 'expired' THEN 2
        WHEN 'canceled' THEN 1
        ELSE 0
      END
      OR (
        CASE t1.`status`
          WHEN 'completed' THEN 5
          WHEN 'opened' THEN 4
          WHEN 'pending' THEN 3
          WHEN 'expired' THEN 2
          WHEN 'canceled' THEN 1
          ELSE 0
        END =
        CASE t2.`status`
          WHEN 'completed' THEN 5
          WHEN 'opened' THEN 4
          WHEN 'pending' THEN 3
          WHEN 'expired' THEN 2
          WHEN 'canceled' THEN 1
          ELSE 0
        END
        AND t1.`id` < t2.`id`
      )
 );

ALTER TABLE `assessment_task`
    DROP INDEX `idx_plan_testee_seq`,
    ADD CONSTRAINT `uk_plan_testee_seq` UNIQUE (`plan_id`, `testee_id`, `seq`);
