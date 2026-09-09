-- SDKMAX Stripe one-time top-up schema rollback for MySQL 5.7.
-- Created: 2026-09-09 20:03 Asia/Shanghai.
-- Scope: local reviewed SQL only. Do not run against production without backup and administrator approval.

-- Preflight: inspect current Stripe-related columns and indexes before rollback.
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

SELECT INDEX_NAME, COLUMN_NAME
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
  DROP INDEX `idx_top_ups_stripe_event_id`,
  DROP INDEX `idx_top_ups_stripe_payment_intent_id`,
  DROP INDEX `idx_top_ups_stripe_recovery_session_id`,
  DROP INDEX `idx_top_ups_stripe_session_id`,
  DROP INDEX `idx_top_ups_currency`;

ALTER TABLE `top_ups`
  DROP COLUMN `payment_error`,
  DROP COLUMN `provider_payload_digest`,
  DROP COLUMN `stripe_livemode`,
  DROP COLUMN `stripe_event_id`,
  DROP COLUMN `stripe_payment_intent_id`,
  DROP COLUMN `stripe_recovery_session_id`,
  DROP COLUMN `stripe_session_id`,
  DROP COLUMN `expected_quota`,
  DROP COLUMN `paid_amount_minor`,
  DROP COLUMN `expected_amount_minor`,
  DROP COLUMN `currency`;

-- Verification: both queries should return zero rows after rollback.
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
