package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func baseAvailability() ModelAvailability {
	return ModelAvailability{
		ModelID:              "openai/example",
		AdminEnabled:         true,
		ConnectivityStatus:   ConnectivityConnected,
		SDKMAXSupportStatus:  SupportSupported,
		APIMode:              APIModeRealtime,
		CustomerVisible:      true,
		VisibilityReason:     VisibilityVisible,
		ConsecutiveSuccesses: 2,
	}
}

func TestModelAvailabilityNewOpenRouterModelDefaultsHidden(t *testing.T) {
	upstream := ModelAvailability{
		ModelID:             "openai/new-model",
		AdminEnabled:        true,
		ConnectivityStatus:  ConnectivityUntested,
		SDKMAXSupportStatus: SupportSupported,
		APIMode:             APIModeRealtime,
	}

	visible, reason := ResolveModelVisibility(upstream)

	if visible {
		t.Fatal("new untested realtime model should be hidden")
	}
	if reason != VisibilityNewModelUntested {
		t.Fatalf("reason = %q, want %q", reason, VisibilityNewModelUntested)
	}
}

func TestModelAvailabilityRealtimeFailureThreshold(t *testing.T) {
	current := baseAvailability()

	first := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{ConnectivityStatus: ConnectivityUpstreamError, FailureReason: "UPSTREAM_ERROR"})
	if !first.CustomerVisible || first.ConnectivityStatus != ConnectivityDegraded {
		t.Fatalf("first failure should stay visible and degraded: %+v", first)
	}
	second := ApplyHealthResultToAvailability(first, ModelHealthCheckResult{ConnectivityStatus: ConnectivityTimeout, FailureReason: "TIMEOUT"})
	if !second.CustomerVisible || second.ConnectivityStatus != ConnectivityDegraded {
		t.Fatalf("second failure should stay visible and degraded: %+v", second)
	}
	third := ApplyHealthResultToAvailability(second, ModelHealthCheckResult{ConnectivityStatus: ConnectivityFailed, FailureReason: "UPSTREAM_ERROR"})
	if third.CustomerVisible || third.ConnectivityStatus != ConnectivityFailed || third.VisibilityReason != VisibilityConnectivityFailed {
		t.Fatalf("third failure should hide model: %+v", third)
	}
}

func TestModelAvailabilitySpecialProtocolsHidden(t *testing.T) {
	cases := []struct {
		name   string
		row    ModelAvailability
		reason string
	}{
		{
			name: "batch unsupported",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityBatchRequired,
				SDKMAXSupportStatus: SupportUnsupported,
				APIMode:             APIModeBatch,
			},
			reason: VisibilityBatchNotSupported,
		},
		{
			name: "image unverified",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityUntested,
				SDKMAXSupportStatus: SupportUnverified,
				APIMode:             APIModeImage,
			},
			reason: VisibilitySdkmaxUnverified,
		},
		{
			name: "wrong endpoint",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityWrongEndpoint,
				SDKMAXSupportStatus: SupportSupported,
				APIMode:             APIModeRealtime,
			},
			reason: VisibilityWrongEndpoint,
		},
		{
			name: "wrong payload",
			row: ModelAvailability{
				AdminEnabled:        true,
				ConnectivityStatus:  ConnectivityWrongPayload,
				SDKMAXSupportStatus: SupportSupported,
				APIMode:             APIModeRealtime,
			},
			reason: VisibilityWrongPayload,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			visible, reason := ResolveModelVisibility(tt.row)
			if visible || reason != tt.reason {
				t.Fatalf("ResolveModelVisibility() = (%v, %q), want (false, %q)", visible, reason, tt.reason)
			}
		})
	}
}

func TestModelAvailabilityRecoveryNeedsTwoSuccesses(t *testing.T) {
	current := baseAvailability()
	current.CustomerVisible = false
	current.ConnectivityStatus = ConnectivityFailed
	current.ConsecutiveFailures = 3
	current.ConsecutiveSuccesses = 0

	first := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{Success: true})
	if first.CustomerVisible || first.VisibilityReason != VisibilityRecoveryPending {
		t.Fatalf("first recovery success should stay hidden: %+v", first)
	}
	second := ApplyHealthResultToAvailability(first, ModelHealthCheckResult{Success: true})
	if !second.CustomerVisible || second.VisibilityReason != VisibilityVisible {
		t.Fatalf("second recovery success should show model: %+v", second)
	}
}

func TestModelAvailabilityAdminDisabledWins(t *testing.T) {
	current := baseAvailability()
	current.AdminEnabled = false
	current.CustomerVisible = false

	next := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{Success: true})
	if next.CustomerVisible || next.VisibilityReason != VisibilityAdminDisabled {
		t.Fatalf("admin disabled model must not auto recover: %+v", next)
	}
}

func TestModelAvailabilityRateLimitAndNoProviderStayDegraded(t *testing.T) {
	for _, status := range []string{ConnectivityRateLimited, ConnectivityNoProvider} {
		current := baseAvailability()
		next := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{ConnectivityStatus: status, FailureReason: status, HTTPStatus: 429})
		if !next.CustomerVisible || next.ConnectivityStatus != ConnectivityDegraded {
			t.Fatalf("%s should stay visible and degraded: %+v", status, next)
		}
	}
}

func TestModelAvailabilityMixedTransientFailuresDoNotSpendRealStrikes(t *testing.T) {
	current := baseAvailability()

	first := ApplyHealthResultToAvailability(current, ModelHealthCheckResult{ConnectivityStatus: ConnectivityRateLimited, FailureReason: "RATE_LIMITED", HTTPStatus: 429})
	second := ApplyHealthResultToAvailability(first, ModelHealthCheckResult{ConnectivityStatus: ConnectivityRateLimited, FailureReason: "RATE_LIMITED", HTTPStatus: 429})
	third := ApplyHealthResultToAvailability(second, ModelHealthCheckResult{ConnectivityStatus: ConnectivityTimeout, FailureReason: "TIMEOUT"})

	if !third.CustomerVisible {
		t.Fatalf("two rate limits followed by one timeout should stay visible: %+v", third)
	}
	if third.ConnectivityStatus != ConnectivityDegraded {
		t.Fatalf("status = %q, want %q", third.ConnectivityStatus, ConnectivityDegraded)
	}
	if third.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures = %d, want 3", third.ConsecutiveFailures)
	}
	if third.ConsecutiveRealFailures != 1 {
		t.Fatalf("ConsecutiveRealFailures = %d, want 1", third.ConsecutiveRealFailures)
	}
}

func TestModelAutoVisibilityDefaultEnabled(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	old := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = old
		common.OptionMapRWMutex.Unlock()
	})

	if !ModelAutoVisibilityEnabled() {
		t.Fatal("auto visibility should default to enabled")
	}
}

func TestBootstrapModelAvailabilityFromAuditCSVParsesRealHeaders(t *testing.T) {
	db := setupAvailabilityTestDB(t)
	csvPath := filepath.Join(t.TempDir(), "audit.csv")
	content := "\ufeffmodel_id,original_status,http_status,original_error,primary_failure_reason,capabilities,api_mode,is_batch,is_image_generation,is_video_generation,current_test_endpoint,expected_endpoint,real_failure,recommend_hide,recommend_retest,notes\n" +
		"openai/good,FAILED,404,No endpoints found,NO_PROVIDER,text,realtime,false,false,false,/v1/chat/completions,/v1/chat/completions,false,false,true,retest\n" +
		"openai/batch:batch,FAILED,400,batch only,BATCH_API_REQUIRED,text,batch,true,false,false,/v1/chat/completions,/v1/batches,false,true,true,batch\n" +
		"openai/broken,FAILED,500,upstream failed,UPSTREAM_ERROR,text,realtime,false,false,false,/v1/chat/completions,/v1/chat/completions,true,true,false,broken\n"
	if err := os.WriteFile(csvPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := BootstrapModelAvailabilityFromAuditCSV(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}

	var noProvider ModelAvailability
	if err := db.Where("model_id = ?", "openai/good").First(&noProvider).Error; err != nil {
		t.Fatal(err)
	}
	if !noProvider.CustomerVisible || noProvider.VisibilityReason != VisibilityNoProviderRetest {
		t.Fatalf("NO_PROVIDER should remain visible for retest: %+v", noProvider)
	}

	var batch ModelAvailability
	if err := db.Where("model_id = ?", "openai/batch:batch").First(&batch).Error; err != nil {
		t.Fatal(err)
	}
	if batch.CustomerVisible || batch.APIMode != APIModeBatch || batch.VisibilityReason != VisibilityBatchNotSupported {
		t.Fatalf("batch should be hidden as unsupported: %+v", batch)
	}

	var broken ModelAvailability
	if err := db.Where("model_id = ?", "openai/broken").First(&broken).Error; err != nil {
		t.Fatal(err)
	}
	if broken.CustomerVisible || broken.ConsecutiveRealFailures != 3 {
		t.Fatalf("real upstream failure should be hidden with real failures: %+v", broken)
	}
}

func TestBootstrapModelAvailabilityFromEmbeddedAuditCSVFallback(t *testing.T) {
	db := setupAvailabilityTestDB(t)
	missingPath := filepath.Join(t.TempDir(), "missing.csv")

	records, err := readModelAvailabilityAuditCSVRecords(missingPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 132 {
		t.Fatalf("embedded records = %d, want 132 including header", len(records))
	}

	count, err := bootstrapModelAvailabilityFromAuditCSVRecords(records)
	if err != nil {
		t.Fatal(err)
	}
	if count != 131 {
		t.Fatalf("count = %d, want 131", count)
	}

	var hidden int64
	if err := db.Model(&ModelAvailability{}).Where("customer_visible = ?", false).Count(&hidden).Error; err != nil {
		t.Fatal(err)
	}
	if hidden == 0 {
		t.Fatal("embedded audit fallback should hide unsupported/problem models")
	}
}

func TestBackfillModelAvailabilityFromEnabledAbilitiesPreservesCurrentVisibleCatalog(t *testing.T) {
	db := setupAvailabilityTestDB(t)
	if err := db.Create(&Ability{Group: "default", Model: "openai/realtime", ChannelId: 1, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Ability{Group: "default", Model: "openai/realtime", ChannelId: 2, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Ability{Group: "default", Model: "openai/gpt-5:batch", ChannelId: 1, Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}

	count, err := BackfillModelAvailabilityFromEnabledAbilities()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 distinct models", count)
	}

	var realtime ModelAvailability
	if err := db.Where("model_id = ?", "openai/realtime").First(&realtime).Error; err != nil {
		t.Fatal(err)
	}
	if !realtime.CustomerVisible || realtime.ConnectivityStatus != ConnectivityConnected {
		t.Fatalf("realtime backfill should be visible connected: %+v", realtime)
	}

	var batch ModelAvailability
	if err := db.Where("model_id = ?", "openai/gpt-5:batch").First(&batch).Error; err != nil {
		t.Fatal(err)
	}
	if batch.CustomerVisible || batch.VisibilityReason != VisibilityBatchNotSupported {
		t.Fatalf("batch backfill should stay hidden: %+v", batch)
	}
}

func TestAdminUpdateWritesOperatorAuditLog(t *testing.T) {
	db := setupAvailabilityTestDB(t)
	if err := db.Create(&ModelAvailability{
		ModelID:              "openai/admin-test",
		Source:               ModelAvailabilitySourceBackfill,
		AdminEnabled:         true,
		ConnectivityStatus:   ConnectivityConnected,
		SDKMAXSupportStatus:  SupportSupported,
		APIMode:              APIModeRealtime,
		CustomerVisible:      true,
		VisibilityReason:     VisibilityVisible,
		ConsecutiveSuccesses: 2,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := SetModelAvailabilityAdminEnabled("openai/admin-test", false, 42, "root")
	if err != nil {
		t.Fatal(err)
	}

	var log ModelAvailabilityAuditLog
	if err := db.Where("model_id = ? AND action = ?", "openai/admin-test", "admin_update").First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.OperatorID != 42 || log.OperatorName != "root" {
		t.Fatalf("operator audit mismatch: %+v", log)
	}
}

func TestRetestRequestPreservesVisibility(t *testing.T) {
	db := setupAvailabilityTestDB(t)
	if err := db.Create(&ModelAvailability{
		ModelID:              "openai/retest-test",
		Source:               ModelAvailabilitySourceBackfill,
		AdminEnabled:         true,
		ConnectivityStatus:   ConnectivityConnected,
		SDKMAXSupportStatus:  SupportSupported,
		APIMode:              APIModeRealtime,
		CustomerVisible:      true,
		VisibilityReason:     VisibilityVisible,
		ConsecutiveSuccesses: 2,
	}).Error; err != nil {
		t.Fatal(err)
	}

	row, err := MarkModelAvailabilityRetestRequested("openai/retest-test", 7, "ops")
	if err != nil {
		t.Fatal(err)
	}
	if !row.CustomerVisible || row.ConnectivityStatus != ConnectivityConnected {
		t.Fatalf("retest request must preserve current visible health: %+v", row)
	}
	if row.LastRetestRequestedByID != 7 || row.LastRetestRequestedByName != "ops" {
		t.Fatalf("retest operator fields mismatch: %+v", row)
	}
}

func setupAvailabilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousLogDB := LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	LOG_DB = db
	if err := db.AutoMigrate(&Ability{}, &ModelAvailability{}, &ModelAvailabilityAuditLog{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
