-- SDKMAX Stripe orphan Checkout Session audit table migration for MySQL 5.7.
-- Created: 2026-09-11 15:18 Asia/Shanghai.
-- Scope: explicit migration only. Do not run against production without backup and administrator approval.

-- Preflight: this query should return zero rows before running CREATE TABLE.
SELECT TABLE_NAME
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits';

CREATE TABLE `stripe_orphan_session_audits` (
  `id` int NOT NULL AUTO_INCREMENT,
  `trade_no` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `user_id` int NOT NULL DEFAULT 0,
  `stripe_session_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `stripe_livemode` tinyint(1) NOT NULL DEFAULT 0,
  `failure_stage` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `attach_error_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `recovery_error_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `expire_error_code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `resolved` tinyint(1) NOT NULL DEFAULT 0,
  `resolved_at` bigint NOT NULL DEFAULT 0,
  `resolution_note` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL DEFAULT 0,
  `updated_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_stripe_orphan_audits_trade_no` (`trade_no`),
  KEY `idx_stripe_orphan_audits_session_id` (`stripe_session_id`),
  KEY `idx_stripe_orphan_audits_resolved` (`resolved`),
  KEY `idx_stripe_orphan_audits_created_at` (`created_at`),
  KEY `idx_stripe_orphan_audits_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Verification: confirm all expected columns exist.
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits'
  AND COLUMN_NAME IN (
    'id',
    'trade_no',
    'user_id',
    'stripe_session_id',
    'stripe_livemode',
    'failure_stage',
    'attach_error_code',
    'recovery_error_code',
    'expire_error_code',
    'resolved',
    'resolved_at',
    'resolution_note',
    'created_at',
    'updated_at'
  )
ORDER BY ORDINAL_POSITION;

-- Verification: confirm required indexes exist and no unnecessary unique index was added.
SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'stripe_orphan_session_audits'
  AND INDEX_NAME IN (
    'idx_stripe_orphan_audits_trade_no',
    'idx_stripe_orphan_audits_session_id',
    'idx_stripe_orphan_audits_resolved',
    'idx_stripe_orphan_audits_created_at',
    'idx_stripe_orphan_audits_user_id'
  )
ORDER BY INDEX_NAME, SEQ_IN_INDEX;
