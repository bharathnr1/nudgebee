-- migrate:no-transaction
-- Recreate the two migration-managed indexes dropped in the up migration
-- (original definitions from V738 and V714). CONCURRENTLY + no-transaction so
-- the rebuild does not lock writes on the recommendation table.
--
-- idx_recommendation_security_acct_image_full is intentionally NOT recreated: it
-- was an INVALID/not-ready stub from a failed concurrent build, never a valid part
-- of the schema and never created by a migration, so there is nothing to restore.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recommendation_security_status_weight
ON recommendation (
    cloud_account_id,
    status,
    (CASE WHEN severity = 'Critical' THEN 10
          WHEN severity = 'High'     THEN 8
          WHEN severity = 'Medium'   THEN 5
          WHEN severity = 'Low'      THEN 2
          WHEN severity = 'Info'     THEN 1
          ELSE 0 END) DESC,
    updated_at DESC
)
WHERE category = 'Security'
  AND rule_name = 'image_scan'
  AND account_object_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS recommendation_dedupe_group_idx
    ON recommendation (cloud_account_id, dedupe_group, category)
    WHERE dedupe_group IS NOT NULL;
