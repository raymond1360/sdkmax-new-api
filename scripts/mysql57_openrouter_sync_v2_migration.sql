-- SDKMAX OpenRouter Model Sync System V2.0
-- MySQL 5.7 compatible migration.
-- Safe to run multiple times.

CREATE TABLE IF NOT EXISTS `open_router_models` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `model_id` varchar(255) NOT NULL,
  `model_name` varchar(255) NOT NULL,
  `provider` varchar(128) DEFAULT '',
  `context_length` bigint DEFAULT 0,
  `prompt_price` decimal(24,12) DEFAULT NULL,
  `completion_price` decimal(24,12) DEFAULT NULL,
  `input_price` decimal(24,12) DEFAULT NULL,
  `output_price` decimal(24,12) DEFAULT NULL,
  `open_router_input_price` varchar(64) NOT NULL DEFAULT '0',
  `open_router_output_price` varchar(64) NOT NULL DEFAULT '0',
  `sdkmax_input_price` varchar(64) NOT NULL DEFAULT '0',
  `sdkmax_output_price` varchar(64) NOT NULL DEFAULT '0',
  `global_multiplier` varchar(16) NOT NULL DEFAULT '1.00',
  `model_multiplier` varchar(16) NOT NULL DEFAULT '1.00',
  `unified_open_router_channel_id` bigint DEFAULT 0,
  `healthy` tinyint(1) DEFAULT 1,
  `health_message` varchar(255) DEFAULT '',
  `last_synced_at` bigint DEFAULT 0,
  `last_health_checked_at` bigint DEFAULT 0,
  `created_time` bigint DEFAULT 0,
  `updated_time` bigint DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_open_router_models_model_id` (`model_id`),
  KEY `idx_open_router_models_provider` (`provider`),
  KEY `idx_open_router_models_channel` (`unified_open_router_channel_id`),
  KEY `idx_open_router_models_healthy` (`healthy`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'model_name') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `model_name` varchar(255) NOT NULL DEFAULT ''''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'provider') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `provider` varchar(128) DEFAULT ''''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'context_length') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `context_length` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'open_router_input_price') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `open_router_input_price` varchar(64) NOT NULL DEFAULT ''0''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'open_router_output_price') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `open_router_output_price` varchar(64) NOT NULL DEFAULT ''0''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'sdkmax_input_price') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `sdkmax_input_price` varchar(64) NOT NULL DEFAULT ''0''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'sdkmax_output_price') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `sdkmax_output_price` varchar(64) NOT NULL DEFAULT ''0''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'global_multiplier') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `global_multiplier` varchar(16) NOT NULL DEFAULT ''1.00''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'model_multiplier') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `model_multiplier` varchar(16) NOT NULL DEFAULT ''1.00''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'unified_open_router_channel_id') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `unified_open_router_channel_id` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'healthy') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `healthy` tinyint(1) DEFAULT 1',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'health_message') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `health_message` varchar(255) DEFAULT ''''',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'last_synced_at') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `last_synced_at` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'last_health_checked_at') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `last_health_checked_at` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'created_time') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `created_time` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND COLUMN_NAME = 'updated_time') = 0,
  'ALTER TABLE `open_router_models` ADD COLUMN `updated_time` bigint DEFAULT 0',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_model_id') = 0,
  'ALTER TABLE `open_router_models` ADD UNIQUE INDEX `idx_open_router_models_model_id` (`model_id`)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Clean up historical V1 duplicate indexes after the V2 model_id index exists.
-- MySQL 5.7 has no DROP INDEX IF EXISTS, so keep these idempotent.
SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_model_id') > 0
  AND (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'model_id') > 0,
  'ALTER TABLE `open_router_models` DROP INDEX `model_id`',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_model_id') > 0
  AND (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'model_id_2') > 0,
  'ALTER TABLE `open_router_models` DROP INDEX `model_id_2`',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_model_id') > 0
  AND (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'model_id_3') > 0,
  'ALTER TABLE `open_router_models` DROP INDEX `model_id_3`',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_provider') = 0,
  'ALTER TABLE `open_router_models` ADD INDEX `idx_open_router_models_provider` (`provider`)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_channel') = 0,
  'ALTER TABLE `open_router_models` ADD INDEX `idx_open_router_models_channel` (`unified_open_router_channel_id`)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql := IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'open_router_models' AND INDEX_NAME = 'idx_open_router_models_healthy') = 0,
  'ALTER TABLE `open_router_models` ADD INDEX `idx_open_router_models_healthy` (`healthy`)',
  'SELECT 1'
);
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS `open_router_model_multipliers` (
  `model_id` varchar(255) NOT NULL,
  `multiplier` varchar(16) NOT NULL DEFAULT '1.00',
  `created_time` bigint DEFAULT 0,
  `updated_time` bigint DEFAULT 0,
  PRIMARY KEY (`model_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `open_router_sync_logs` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `status` varchar(32) DEFAULT '',
  `message` text,
  `models_fetched` bigint DEFAULT 0,
  `models_created` bigint DEFAULT 0,
  `models_updated` bigint DEFAULT 0,
  `models_disabled` bigint DEFAULT 0,
  `channel_id` bigint DEFAULT 0,
  `started_at` bigint DEFAULT 0,
  `finished_at` bigint DEFAULT 0,
  `duration_ms` bigint DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_open_router_sync_logs_status` (`status`),
  KEY `idx_open_router_sync_logs_channel` (`channel_id`),
  KEY `idx_open_router_sync_logs_started_at` (`started_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO `options` (`key`, `value`)
SELECT 'OpenRouterGlobalPriceMultiplier', '1.00'
WHERE NOT EXISTS (SELECT 1 FROM `options` WHERE `key` = 'OpenRouterGlobalPriceMultiplier');

INSERT INTO `options` (`key`, `value`)
SELECT 'OpenRouterUnifiedChannelId', '0'
WHERE NOT EXISTS (SELECT 1 FROM `options` WHERE `key` = 'OpenRouterUnifiedChannelId');

INSERT INTO `options` (`key`, `value`)
SELECT 'OpenRouterAutoSyncEnabled', 'false'
WHERE NOT EXISTS (SELECT 1 FROM `options` WHERE `key` = 'OpenRouterAutoSyncEnabled');

INSERT INTO `options` (`key`, `value`)
SELECT 'OpenRouterAutoSyncIntervalMinutes', '360'
WHERE NOT EXISTS (SELECT 1 FROM `options` WHERE `key` = 'OpenRouterAutoSyncIntervalMinutes');

UPDATE `open_router_models`
SET
  `open_router_input_price` = CAST(`prompt_price` AS CHAR),
  `open_router_output_price` = CAST(`completion_price` AS CHAR),
  `sdkmax_input_price` = CAST(
    CAST(`prompt_price` AS DECIMAL(24,12))
    * CAST((SELECT `value` FROM `options` WHERE `key` = 'OpenRouterGlobalPriceMultiplier' LIMIT 1) AS DECIMAL(10,4))
    * CAST(`model_multiplier` AS DECIMAL(10,4))
    AS CHAR
  ),
  `sdkmax_output_price` = CAST(
    CAST(`completion_price` AS DECIMAL(24,12))
    * CAST((SELECT `value` FROM `options` WHERE `key` = 'OpenRouterGlobalPriceMultiplier' LIMIT 1) AS DECIMAL(10,4))
    * CAST(`model_multiplier` AS DECIMAL(10,4))
    AS CHAR
  )
WHERE (`open_router_input_price` = '0' OR `open_router_output_price` = '0')
  AND `prompt_price` IS NOT NULL
  AND `completion_price` IS NOT NULL;
