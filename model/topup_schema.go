package model

import (
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const topUpTableName = "top_ups"

var (
	stripeTopUpSchemaMu       sync.RWMutex
	stripeTopUpSchemaReady    bool
	stripeTopUpSchemaLastInfo string
)

var requiredStripeTopUpColumns = []string{
	"currency",
	"expected_amount_minor",
	"paid_amount_minor",
	"expected_quota",
	"stripe_session_id",
	"stripe_recovery_session_id",
	"stripe_payment_intent_id",
	"stripe_event_id",
	"stripe_livemode",
	"provider_payload_digest",
	"payment_error",
}

var requiredStripeTopUpIndexes = []string{
	"idx_top_ups_stripe_session_id",
	"idx_top_ups_stripe_event_id",
}

func ensureTopUpTableForMigration() error {
	if DB == nil {
		setStripeTopUpSchemaReadiness(false, "database is not initialized")
		return nil
	}
	if !DB.Migrator().HasTable(&TopUp{}) {
		if err := DB.AutoMigrate(&TopUp{}); err != nil {
			setStripeTopUpSchemaReadiness(false, "failed to create top_ups table")
			return err
		}
	}
	return CheckStripeTopUpSchemaReadiness(DB)
}

func CheckStripeTopUpSchemaReadiness(db *gorm.DB) error {
	if db == nil {
		setStripeTopUpSchemaReadiness(false, "database is not initialized")
		return nil
	}
	if !db.Migrator().HasTable(&TopUp{}) {
		setStripeTopUpSchemaReadiness(false, "top_ups table is missing")
		return nil
	}

	missingColumns := make([]string, 0)
	for _, column := range requiredStripeTopUpColumns {
		if !db.Migrator().HasColumn(&TopUp{}, column) {
			missingColumns = append(missingColumns, column)
		}
	}

	missingIndexes := make([]string, 0)
	for _, index := range requiredStripeTopUpIndexes {
		if !hasTopUpIndex(db, index) {
			missingIndexes = append(missingIndexes, index)
		}
	}

	if len(missingColumns) > 0 || len(missingIndexes) > 0 {
		parts := []string{}
		if len(missingColumns) > 0 {
			parts = append(parts, "missing columns: "+strings.Join(missingColumns, ","))
		}
		if len(missingIndexes) > 0 {
			parts = append(parts, "missing indexes: "+strings.Join(missingIndexes, ","))
		}
		info := strings.Join(parts, "; ")
		setStripeTopUpSchemaReadiness(false, info)
		common.SysLog("Stripe database migration not ready for top_ups: " + info)
		return nil
	}

	setStripeTopUpSchemaReadiness(true, "ready")
	return nil
}

func hasTopUpIndex(db *gorm.DB, indexName string) bool {
	if common.UsingSQLite {
		var count int64
		if err := db.Raw("SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND name = ?", topUpTableName, indexName).Scan(&count).Error; err != nil {
			return false
		}
		return count > 0
	}
	if common.UsingPostgreSQL {
		var count int64
		if err := db.Raw(`SELECT COUNT(1)
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = ?
			  AND indexname = ?`, topUpTableName, indexName).Scan(&count).Error; err != nil {
			return false
		}
		return count > 0
	}
	if common.UsingMySQL {
		var count int64
		if err := db.Raw(`SELECT COUNT(1)
			FROM information_schema.statistics
			WHERE table_schema = DATABASE()
			  AND table_name = ?
			  AND index_name = ?`, topUpTableName, indexName).Scan(&count).Error; err != nil {
			return false
		}
		return count > 0
	}
	return db.Migrator().HasIndex(&TopUp{}, indexName)
}

func setStripeTopUpSchemaReadiness(ready bool, info string) {
	stripeTopUpSchemaMu.Lock()
	defer stripeTopUpSchemaMu.Unlock()
	stripeTopUpSchemaReady = ready
	stripeTopUpSchemaLastInfo = info
}

func SetStripeTopUpSchemaReadinessForTest(ready bool, info string) {
	setStripeTopUpSchemaReadiness(ready, info)
}

func IsStripeTopUpSchemaReady() bool {
	stripeTopUpSchemaMu.RLock()
	defer stripeTopUpSchemaMu.RUnlock()
	return stripeTopUpSchemaReady
}

func StripeTopUpSchemaReadinessInfo() string {
	stripeTopUpSchemaMu.RLock()
	defer stripeTopUpSchemaMu.RUnlock()
	if stripeTopUpSchemaLastInfo == "" {
		return "not checked"
	}
	return stripeTopUpSchemaLastInfo
}
