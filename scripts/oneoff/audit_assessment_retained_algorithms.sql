-- audit_assessment_retained_algorithms.sql
-- MC-R018 batch 4: inventory Assessment / Outcome historical model_algorithm
-- retained-read aliases. Read-only. Does NOT rewrite rows.
--
-- Usage (replace DSN placeholders; never paste real passwords into chat):
--   mysql -h HOST -u USER -p qs < scripts/oneoff/audit_assessment_retained_algorithms.sql

-- Assessment: typology retained aliases + behavioral_rating_default + empty algorithm
SELECT 'assessment' AS source,
       evaluation_model_kind AS kind,
       evaluation_model_algorithm AS algorithm,
       COUNT(*) AS cnt
FROM assessment
WHERE deleted_at IS NULL
  AND (
    evaluation_model_algorithm IN ('mbti', 'sbti', 'bigfive', 'behavioral_rating_default')
    OR evaluation_model_algorithm IS NULL
    OR evaluation_model_algorithm = ''
  )
GROUP BY evaluation_model_kind, evaluation_model_algorithm
ORDER BY cnt DESC;

-- Evaluation outcome: same retained aliases (no soft-delete column on this table)
SELECT 'evaluation_outcome' AS source,
       model_kind AS kind,
       model_algorithm AS algorithm,
       COUNT(*) AS cnt
FROM evaluation_outcome
WHERE model_algorithm IN ('mbti', 'sbti', 'bigfive', 'behavioral_rating_default')
   OR model_algorithm IS NULL
   OR model_algorithm = ''
GROUP BY model_kind, model_algorithm
ORDER BY cnt DESC;
