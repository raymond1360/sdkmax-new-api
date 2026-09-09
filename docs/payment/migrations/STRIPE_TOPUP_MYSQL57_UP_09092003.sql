-- SDKMAX Stripe one-time top-up schema migration for MySQL 5.7.
-- Created: 2026-09-09 20:03 Asia/Shanghai.
-- Scope: local reviewed SQL only. Do not run against production without backup and administrator approval.

-- Preflight: every query below must return zero rows before running the ALTER statements.
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'top_ups'
  AND COLUMN_NAME IN (
    'currency',
    'expected_amount_minor',
    'paid_amount_minor',
    'expected_quota',
    'stripe_session_id',
    'stripe_recovery_session_id',
    'stripe_payment_intent_id',
    'stripe_event_id',
    'stripe_livemode',
    'provider_payload_digest',
    'payment_error'
  );

SELECT INDEX_NAME
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'top_ups'
  AND INDEX_NAME IN (
    'idx_top_ups_currency',
    'idx_top_ups_stripe_session_id',
    'idx_top_ups_stripe_recovery_session_id',
    'idx_top_ups_stripe_payment_intent_id',
    'idx_top_ups_stripe_event_id'
  );

ALTER TABLE `top_ups`
  ADD COLUMN `currency` varchar(8) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' AFTER `payment_provider`,
  ADD COLUMN `expected_amount_minor` bigint NOT NULL DEFAULT 0 AFTER `currency`,
  ADD COLUMN `paid_amount_minor` bigint NOT NULL DEFAULT 0 AFTER `expected_amount_minor`,
  ADD COLUMN `expected_quota` int NOT NULL DEFAULT 0 AFTER `paid_amount_minor`,
  ADD COLUMN `stripe_session_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `expected_quota`,
  ADD COLUMN `stripe_recovery_session_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `stripe_session_id`,
  ADD COLUMN `stripe_payment_intent_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `stripe_recovery_session_id`,
  ADD COLUMN `stripe_event_id` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `stripe_payment_intent_id`,
  ADD COLUMN `stripe_livemode` tinyint(1) DEFAULT NULL AFTER `stripe_event_id`,
  ADD COLUMN `provider_payload_digest` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `stripe_livemode`,
  ADD COLUMN `payment_error` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER `provider_payload_digest`;

ALTER TABLE `top_ups`
  ADD INDEX `idx_top_ups_currency` (`currency`),
  ADD UNIQUE INDEX `idx_top_ups_stripe_session_id` (`stripe_session_id`),
  ADD INDEX `idx_top_ups_stripe_recovery_session_id` (`stripe_recovery_session_id`),
  ADD INDEX `idx_top_ups_stripe_payment_intent_id` (`stripe_payment_intent_id`),
  ADD UNIQUE INDEX `idx_top_ups_stripe_event_id` (`stripe_event_id`);

-- Verification: confirm all new columns and indexes exist after migration.
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'top_ups'
  AND COLUMN_NAME IN (
    'currency',
    'expected_amount_minor',
    'paid_amount_minor',
    'expected_quota',
    'stripe_session_id',
    'stripe_recovery_session_id',
    'stripe_payment_intent_id',
    'stripe_event_id',
    'stripe_livemode',
    'provider_payload_digest',
    'payment_error'
  )
ORDER BY ORDINAL_POSITION;

SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'top_ups'
  AND INDEX_NAME IN (
    'idx_top_ups_currency',
    'idx_top_ups_stripe_session_id',
    'idx_top_ups_stripe_recovery_session_id',
    'idx_top_ups_stripe_payment_intent_id',
    'idx_top_ups_stripe_event_id'
  )
ORDER BY INDEX_NAME, SEQ_IN_INDEX;
