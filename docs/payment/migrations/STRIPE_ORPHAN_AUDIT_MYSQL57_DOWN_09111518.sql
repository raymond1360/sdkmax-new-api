-- SDKMAX Stripe orphan Checkout Session audit table rollback for MySQL 5.7.
-- Created: 2026-09-11 15:18 Asia/Shanghai.
-- Scope: explicit rollback only. Do not run against production without backup and administrator approval.
-- Rollback should only be run after the code version that depends on this table has been reverted.

-- Preflight: inspect the table before rollback.
SELECT TABLE_NAME
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits';

SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits'
ORDER BY INDEX_NAME, SEQ_IN_INDEX;

DROP TABLE `stripe_orphan_session_audits`;

-- Verification: this query should return zero rows after rollback.
SELECT TABLE_NAME
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits';
