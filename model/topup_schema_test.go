package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyTopUpForSchemaTest struct {
	Id              int `gorm:"primaryKey"`
	UserId          int
	Amount          int64
	Money           float64
	TradeNo         string `gorm:"uniqueIndex"`
	PaymentMethod   string
	PaymentProvider string
	CreateTime      int64
	Status          string
}

func (legacyTopUpForSchemaTest) TableName() string {
	return "top_ups"
}

func newTopUpSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))), &gorm.Config{})
	require.NoError(t, err)
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		setStripeTopUpSchemaReadiness(false, "not checked")
	})
	return db
}

func withTopUpSchemaTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })
}

func TestTopUpSchemaReadinessExistingLegacyTableDoesNotAutoAlter(t *testing.T) {
	db := newTopUpSchemaTestDB(t)
	withTopUpSchemaTestDB(t, db)
	require.NoError(t, db.AutoMigrate(&legacyTopUpForSchemaTest{}))
	require.False(t, db.Migrator().HasColumn(&TopUp{}, "stripe_session_id"))

	require.NoError(t, ensureTopUpTableForMigration())

	assert.False(t, IsStripeTopUpSchemaReady())
	assert.Contains(t, StripeTopUpSchemaReadinessInfo(), "missing columns")
	assert.False(t, db.Migrator().HasColumn(&TopUp{}, "stripe_session_id"))
	assert.False(t, hasTopUpIndex(db, "idx_top_ups_stripe_session_id"))
}

func TestTopUpSchemaReadinessReadyTableEnablesStripeSchema(t *testing.T) {
	db := newTopUpSchemaTestDB(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))

	require.NoError(t, CheckStripeTopUpSchemaReadiness(db))

	assert.True(t, IsStripeTopUpSchemaReady())
	assert.Equal(t, "ready", StripeTopUpSchemaReadinessInfo())
}

func TestTopUpSchemaReadinessFreshDatabaseCreatesTopUpsTable(t *testing.T) {
	db := newTopUpSchemaTestDB(t)
	withTopUpSchemaTestDB(t, db)
	require.False(t, db.Migrator().HasTable(&TopUp{}))

	require.NoError(t, ensureTopUpTableForMigration())

	assert.True(t, db.Migrator().HasTable(&TopUp{}))
	assert.True(t, db.Migrator().HasColumn(&TopUp{}, "expected_quota"))
	assert.True(t, hasTopUpIndex(db, "idx_top_ups_stripe_session_id"))
	assert.True(t, IsStripeTopUpSchemaReady())
}

func TestTopUpSchemaReadinessCheckIsIdempotent(t *testing.T) {
	db := newTopUpSchemaTestDB(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))

	require.NoError(t, CheckStripeTopUpSchemaReadiness(db))
	require.NoError(t, CheckStripeTopUpSchemaReadiness(db))

	assert.True(t, IsStripeTopUpSchemaReady())
	assert.Equal(t, "ready", StripeTopUpSchemaReadinessInfo())
}

func TestTopUpSchemaReadinessNilDBDoesNotEnableStripe(t *testing.T) {
	require.NoError(t, CheckStripeTopUpSchemaReadiness(nil))

	assert.False(t, IsStripeTopUpSchemaReady())
	assert.Equal(t, "database is not initialized", StripeTopUpSchemaReadinessInfo())
}
