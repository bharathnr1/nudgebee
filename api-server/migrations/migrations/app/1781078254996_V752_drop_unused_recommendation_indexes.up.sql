-- migrate:no-transaction
-- Drop three unused indexes on the 60 GB `recommendation` table to cut
-- autovacuum index-vacuum time (every cycle scans all indexes serially).
-- All three confirmed unused on prod (idx_scan = 0 over the full stats window):
--   * idx_recommendation_security_status_weight  (654 MB, V738) — partial image_scan
--     severity-weight ordering index; planner prefers idx_recommendation_security_account_image_name.
--   * recommendation_dedupe_group_idx             (40 kB,  V714) — dedupe_group is null
--     for ~all rows; the dedupe_group column stays, only the index is dropped.
--   * idx_recommendation_security_acct_image_full (0 bytes)       — INVALID/not-ready stub
--     left by a failed CREATE INDEX CONCURRENTLY; never created by a migration.
--
-- CONCURRENTLY avoids the brief ACCESS EXCLUSIVE lock a plain DROP INDEX would take
-- on this hot table; requires running outside a transaction (migrate:no-transaction above).
-- DROP INDEX CONCURRENTLY drops one index per statement (no CASCADE, no multi-drop).

DROP INDEX CONCURRENTLY IF EXISTS idx_recommendation_security_status_weight;

DROP INDEX CONCURRENTLY IF EXISTS recommendation_dedupe_group_idx;

DROP INDEX CONCURRENTLY IF EXISTS idx_recommendation_security_acct_image_full;
