-- Fix: Remove duplicate assessment records caused by race condition
-- between Submit() and scanner() in AssessmentWorker.
-- Keeps the earliest record per (usage_log_id, assessment_layer).

BEGIN;

-- 1. Delete duplicate assessment records
DELETE FROM skill_assessment_results
WHERE id IN (
  SELECT id FROM (
    SELECT id, ROW_NUMBER() OVER (
      PARTITION BY usage_log_id, assessment_layer
      ORDER BY assessed_at ASC
    ) as rn
    FROM skill_assessment_results
  ) sub
  WHERE rn > 1
);

-- 2. Add unique constraint to prevent future duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_assessment_usage_log_layer_unique
  ON skill_assessment_results (usage_log_id, assessment_layer);

-- 3. Reset any stuck "assessing" status back to "pending"
-- (safety net for the new CAS mechanism)
UPDATE skill_usage_logs
SET assessment_status = 'pending'
WHERE assessment_status = 'assessing';

COMMIT;
