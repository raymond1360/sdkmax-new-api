package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetCustomerVisibleGroupModelsFiltersVisibilityAndModelStatus(t *testing.T) {
	db := setupAbilityVisibilityTestDB(t, true)
	createAbilityVisibilityFixture(t, db)

	models := GetCustomerVisibleGroupModels("default")

	assertSameStrings(t, models, []string{
		"openai/visible",
		"openai/no-availability",
		"openai/no-metadata",
	})
}

func TestGetCustomerVisibleGroupModelsAllowsWhenAvailabilityTableMissing(t *testing.T) {
	db := setupAbilityVisibilityTestDB(t, false)
	createAbility(t, db, "default", "openai/no-availability-table", 1, true)

	models := GetCustomerVisibleGroupModels("default")

	assertSameStrings(t, models, []string{"openai/no-availability-table"})
}

func TestGetAllEnableAbilityWithChannelsUsesSharedVisibilityScope(t *testing.T) {
	db := setupAbilityVisibilityTestDB(t, true)
	createAbilityVisibilityFixture(t, db)

	abilities, err := GetAllEnableAbilityWithChannels()
	if err != nil {
		t.Fatal(err)
	}

	var models []string
	for _, ability := range abilities {
		models = append(models, ability.Model)
	}
	assertSameStrings(t, models, []string{
		"openai/visible",
		"openai/no-availability",
		"openai/no-metadata",
		"openai/vip-only",
	})
}

func setupAbilityVisibilityTestDB(t *testing.T, migrateAvailability bool) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousGroupCol := commonGroupCol
	previousKeyCol := commonKeyCol
	previousTrueVal := commonTrueVal
	previousFalseVal := commonFalseVal

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	LOG_DB = db
	modelAvailabilityTableExistsCache = sync.Map{}

	tables := []any{&Ability{}, &Channel{}, &Model{}}
	if migrateAvailability {
		tables = append(tables, &ModelAvailability{})
	}
	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		commonGroupCol = previousGroupCol
		commonKeyCol = previousKeyCol
		commonTrueVal = previousTrueVal
		commonFalseVal = previousFalseVal
		modelAvailabilityTableExistsCache = sync.Map{}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createAbilityVisibilityFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	createAbility(t, db, "default", "openai/visible", 1, true)
	createAbility(t, db, "default", "openai/hidden", 2, true)
	createAbility(t, db, "default", "openai/disabled-metadata", 3, true)
	createAbility(t, db, "default", "openai/no-availability", 4, true)
	createAbility(t, db, "default", "openai/no-metadata", 5, true)
	createAbility(t, db, "default", "openai/disabled-ability", 6, false)
	createAbility(t, db, "vip", "openai/vip-only", 7, true)
	createModelMeta(t, db, "openai/visible", 1)
	createModelMeta(t, db, "openai/hidden", 1)
	createModelMeta(t, db, "openai/disabled-metadata", 0)
	createModelMeta(t, db, "openai/no-availability", 1)
	createModelAvailability(t, db, "openai/visible", true)
	createModelAvailability(t, db, "openai/hidden", false)
	createModelAvailability(t, db, "openai/disabled-metadata", true)
}

func createAbility(t *testing.T, db *gorm.DB, group string, modelName string, channelID int, enabled bool) {
	t.Helper()
	if err := db.FirstOrCreate(&Channel{Id: channelID}, Channel{Id: channelID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: channelID,
		Enabled:   enabled,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func createModelMeta(t *testing.T, db *gorm.DB, modelName string, status int) {
	t.Helper()
	if err := db.Create(&Model{ModelName: modelName, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if status != 1 {
		if err := db.Model(&Model{}).Where("model_name = ?", modelName).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func createModelAvailability(t *testing.T, db *gorm.DB, modelName string, visible bool) {
	t.Helper()
	if err := db.Create(&ModelAvailability{
		ModelID:              modelName,
		AdminEnabled:         true,
		ConnectivityStatus:   ConnectivityConnected,
		SDKMAXSupportStatus:  SupportSupported,
		APIMode:              APIModeRealtime,
		CustomerVisible:      visible,
		VisibilityReason:     VisibilityVisible,
		ConsecutiveSuccesses: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func assertSameStrings(t *testing.T, actual []string, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("models = %v, want %v", actual, expected)
	}
	counts := make(map[string]int, len(expected))
	for _, value := range actual {
		counts[value]++
	}
	for _, value := range expected {
		counts[value]--
	}
	for value, count := range counts {
		if count != 0 {
			t.Fatalf("models = %v, want %v; mismatch at %s", actual, expected, value)
		}
	}
}
