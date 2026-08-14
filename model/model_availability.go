package model

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed audit_data/model-failure-reclassification-202608.csv
var embeddedModelAvailabilityAuditCSV []byte

const (
	ModelAvailabilitySourceOpenRouter = "openrouter"
	ModelAvailabilitySourceAudit      = "audit"
	ModelAvailabilitySourceBackfill   = "backfill"

	ConnectivityUnknown         = "unknown"
	ConnectivityUntested        = "untested"
	ConnectivityTesting         = "testing"
	ConnectivityConnected       = "connected"
	ConnectivityDegraded        = "degraded"
	ConnectivityFailed          = "failed"
	ConnectivityWrongEndpoint   = "wrong_endpoint"
	ConnectivityWrongPayload    = "wrong_payload"
	ConnectivityBatchRequired   = "batch_required"
	ConnectivityUnsupported     = "unsupported"
	ConnectivityUpstreamMissing = "upstream_missing"
	ConnectivityRateLimited     = "rate_limited"
	ConnectivityNoProvider      = "no_provider"
	ConnectivityTimeout         = "timeout"
	ConnectivityUpstreamError   = "upstream_error"

	SupportSupported   = "supported"
	SupportUnsupported = "unsupported"
	SupportUnverified  = "unverified"

	APIModeRealtime  = "realtime"
	APIModeBatch     = "batch"
	APIModeAsync     = "async"
	APIModeImage     = "image"
	APIModeVideo     = "video"
	APIModeAudio     = "audio"
	APIModeEmbedding = "embedding"
	APIModeSearch    = "search"

	VisibilityVisible              = "VISIBLE"
	VisibilityAdminDisabled        = "ADMIN_DISABLED"
	VisibilityBatchNotSupported    = "BATCH_NOT_SUPPORTED"
	VisibilitySdkmaxUnsupported    = "SDKMAX_UNSUPPORTED"
	VisibilitySdkmaxUnverified     = "SDKMAX_UNVERIFIED"
	VisibilityNewModelUntested     = "NEW_MODEL_UNTESTED"
	VisibilityHealthCheckPending   = "HEALTH_CHECK_PENDING"
	VisibilityConnectivityFailed   = "CONNECTIVITY_FAILED"
	VisibilityRecoveryPending      = "RECOVERY_PENDING"
	VisibilityWrongEndpoint        = "WRONG_ENDPOINT"
	VisibilityWrongPayload         = "WRONG_PAYLOAD"
	VisibilityUpstreamModelMissing = "UPSTREAM_MODEL_MISSING"
	VisibilityRateLimitedRetest    = "RATE_LIMITED_RETEST"
	VisibilityNoProviderRetest     = "NO_PROVIDER_RETEST"
	VisibilityRetestQueued         = "RETEST_QUEUED"
)

type ModelAvailability struct {
	Id                        int    `json:"id"`
	ModelID                   string `json:"model_id" gorm:"type:varchar(255);uniqueIndex;not null"`
	Source                    string `json:"source" gorm:"type:varchar(64);index;not null;default:''"`
	AdminEnabled              bool   `json:"admin_enabled" gorm:"not null;default:true"`
	ConnectivityStatus        string `json:"connectivity_status" gorm:"type:varchar(64);index;not null;default:'unknown'"`
	SDKMAXSupportStatus       string `json:"sdkmax_support_status" gorm:"type:varchar(64);index;not null;default:'unverified'"`
	APIMode                   string `json:"api_mode" gorm:"type:varchar(64);index;not null;default:'realtime'"`
	Capabilities              string `json:"capabilities" gorm:"type:text"`
	CustomerVisible           bool   `json:"customer_visible" gorm:"index;not null;default:false"`
	VisibilityReason          string `json:"visibility_reason" gorm:"type:varchar(128);index;not null;default:''"`
	ConsecutiveFailures       int    `json:"consecutive_failures" gorm:"not null;default:0"`
	ConsecutiveRealFailures   int    `json:"consecutive_real_failures" gorm:"not null;default:0"`
	ConsecutiveSuccesses      int    `json:"consecutive_successes" gorm:"not null;default:0"`
	LastTestedAt              int64  `json:"last_tested_at" gorm:"index"`
	LastSuccessAt             int64  `json:"last_success_at" gorm:"index"`
	LastFailureAt             int64  `json:"last_failure_at" gorm:"index"`
	LastFailureReason         string `json:"last_failure_reason" gorm:"type:varchar(128);index"`
	LastHTTPStatus            int    `json:"last_http_status" gorm:"index"`
	LastError                 string `json:"last_error" gorm:"type:text"`
	LastUpstreamSeenAt        int64  `json:"last_upstream_seen_at" gorm:"index"`
	LastRetestRequestedAt     int64  `json:"last_retest_requested_at" gorm:"index"`
	LastRetestRequestedByID   int    `json:"last_retest_requested_by_id" gorm:"index"`
	LastRetestRequestedByName string `json:"last_retest_requested_by_name" gorm:"type:varchar(128)"`
	CreatedTime               int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime               int64  `json:"updated_time" gorm:"bigint"`
}

type ModelAvailabilityAuditLog struct {
	Id              int    `json:"id"`
	ModelID         string `json:"model_id" gorm:"type:varchar(255);index;not null"`
	Action          string `json:"action" gorm:"type:varchar(64);index;not null"`
	PreviousVisible bool   `json:"previous_visible"`
	NextVisible     bool   `json:"next_visible"`
	PreviousStatus  string `json:"previous_status" gorm:"type:varchar(64)"`
	NextStatus      string `json:"next_status" gorm:"type:varchar(64)"`
	Reason          string `json:"reason" gorm:"type:varchar(128);index"`
	Detail          string `json:"detail" gorm:"type:text"`
	OperatorID      int    `json:"operator_id" gorm:"index"`
	OperatorName    string `json:"operator_name" gorm:"type:varchar(128)"`
	CreatedTime     int64  `json:"created_time" gorm:"bigint;index"`
}

type ModelHealthCheckResult struct {
	ConnectivityStatus string
	FailureReason      string
	HTTPStatus         int
	Error              string
	Success            bool
	TestedAt           int64
}

type ModelAvailabilityUpstream struct {
	ModelID      string
	Source       string
	APIMode      string
	Capabilities []string
	SeenAt       int64
}

func normalizeAvailabilityDefaults(a *ModelAvailability) {
	if a.Source == "" {
		a.Source = ModelAvailabilitySourceOpenRouter
	}
	if a.ConnectivityStatus == "" {
		a.ConnectivityStatus = ConnectivityUnknown
	}
	if a.SDKMAXSupportStatus == "" {
		a.SDKMAXSupportStatus = SupportUnverified
	}
	if a.APIMode == "" {
		a.APIMode = APIModeRealtime
	}
}

func capabilitiesToText(capabilities []string) string {
	seen := map[string]struct{}{}
	clean := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(strings.ToLower(capability))
		if capability == "" {
			continue
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		clean = append(clean, capability)
	}
	return strings.Join(clean, ",")
}

func supportStatusForAPIMode(apiMode string) string {
	switch apiMode {
	case APIModeRealtime, APIModeEmbedding:
		return SupportSupported
	case APIModeBatch:
		return SupportUnsupported
	default:
		return SupportUnverified
	}
}

func ResolveModelVisibility(a ModelAvailability) (bool, string) {
	normalizeAvailabilityDefaults(&a)
	if !a.AdminEnabled {
		return false, VisibilityAdminDisabled
	}
	if a.ConnectivityStatus == ConnectivityUpstreamMissing {
		return false, VisibilityUpstreamModelMissing
	}
	if a.APIMode == APIModeBatch || a.ConnectivityStatus == ConnectivityBatchRequired {
		return false, VisibilityBatchNotSupported
	}
	if a.SDKMAXSupportStatus == SupportUnsupported {
		return false, VisibilitySdkmaxUnsupported
	}
	if a.SDKMAXSupportStatus == SupportUnverified {
		return false, VisibilitySdkmaxUnverified
	}
	switch a.ConnectivityStatus {
	case ConnectivityConnected:
		if a.ConsecutiveSuccesses < 2 {
			return false, VisibilityRecoveryPending
		}
		return true, VisibilityVisible
	case ConnectivityDegraded, ConnectivityRateLimited, ConnectivityNoProvider, ConnectivityTimeout, ConnectivityUpstreamError:
		if (a.ConnectivityStatus == ConnectivityTimeout || a.ConnectivityStatus == ConnectivityUpstreamError) && a.ConsecutiveRealFailures >= 3 {
			return false, VisibilityConnectivityFailed
		}
		return true, a.VisibilityReason
	case ConnectivityWrongEndpoint:
		return false, VisibilityWrongEndpoint
	case ConnectivityWrongPayload:
		return false, VisibilityWrongPayload
	case ConnectivityFailed:
		if a.ConsecutiveRealFailures >= 3 {
			return false, VisibilityConnectivityFailed
		}
		return true, a.VisibilityReason
	case ConnectivityUntested:
		return false, VisibilityNewModelUntested
	case ConnectivityTesting, ConnectivityUnknown:
		return false, VisibilityHealthCheckPending
	default:
		return false, VisibilityHealthCheckPending
	}
}

var modelAvailabilityTableExistsCache sync.Map

func ModelAutoVisibilityEnabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if value, ok := common.OptionMap["MODEL_AUTO_VISIBILITY_ENABLED"]; ok {
		return value == "true"
	}
	value, ok := common.OptionMap["ModelAutoVisibilityEnabled"]
	if !ok {
		return true
	}
	return value == "true"
}

func ModelAvailabilityTableExists() bool {
	if DB == nil {
		return false
	}
	cacheKey := fmt.Sprintf("%p", DB)
	if cached, ok := modelAvailabilityTableExistsCache.Load(cacheKey); ok {
		return cached.(bool)
	}
	exists := DB.Migrator().HasTable(&ModelAvailability{})
	modelAvailabilityTableExistsCache.Store(cacheKey, exists)
	return exists
}

func ApplyHealthResultToAvailability(current ModelAvailability, result ModelHealthCheckResult) ModelAvailability {
	next := current
	normalizeAvailabilityDefaults(&next)
	now := result.TestedAt
	if now == 0 {
		now = common.GetTimestamp()
	}
	next.LastTestedAt = now
	next.LastHTTPStatus = result.HTTPStatus
	next.LastError = result.Error
	if result.Success {
		next.ConnectivityStatus = ConnectivityConnected
		next.ConsecutiveSuccesses++
		next.ConsecutiveFailures = 0
		next.ConsecutiveRealFailures = 0
		next.LastSuccessAt = now
		next.LastFailureReason = ""
		next.LastError = ""
	} else {
		status := strings.TrimSpace(result.ConnectivityStatus)
		if status == "" {
			status = ConnectivityFailed
		}
		next.ConsecutiveFailures++
		next.ConsecutiveSuccesses = 0
		next.LastFailureAt = now
		next.LastFailureReason = strings.TrimSpace(result.FailureReason)
		switch status {
		case ConnectivityUpstreamMissing, ConnectivityWrongEndpoint, ConnectivityWrongPayload, ConnectivityBatchRequired, ConnectivityUnsupported:
			next.ConsecutiveRealFailures = 0
			next.ConnectivityStatus = status
		case ConnectivityRateLimited:
			next.ConsecutiveRealFailures = 0
			next.ConnectivityStatus = ConnectivityDegraded
			next.VisibilityReason = VisibilityRateLimitedRetest
		case ConnectivityNoProvider:
			next.ConsecutiveRealFailures = 0
			next.ConnectivityStatus = ConnectivityDegraded
			next.VisibilityReason = VisibilityNoProviderRetest
		case ConnectivityTimeout, ConnectivityUpstreamError:
			next.ConsecutiveRealFailures++
			if next.ConsecutiveRealFailures >= 3 {
				next.ConnectivityStatus = ConnectivityFailed
			} else {
				next.ConnectivityStatus = ConnectivityDegraded
			}
		default:
			next.ConsecutiveRealFailures++
			if next.ConsecutiveRealFailures >= 3 {
				next.ConnectivityStatus = ConnectivityFailed
			} else {
				next.ConnectivityStatus = ConnectivityDegraded
			}
		}
	}
	next.CustomerVisible, next.VisibilityReason = ResolveModelVisibility(next)
	next.UpdatedTime = now
	return next
}

func UpsertModelAvailabilityFromUpstream(tx *gorm.DB, upstream ModelAvailabilityUpstream) error {
	modelID := strings.TrimSpace(upstream.ModelID)
	if modelID == "" {
		return nil
	}
	useDB := DB
	if tx != nil {
		useDB = tx
	} else {
		return DB.Transaction(func(tx *gorm.DB) error {
			return UpsertModelAvailabilityFromUpstream(tx, upstream)
		})
	}
	now := upstream.SeenAt
	if now == 0 {
		now = common.GetTimestamp()
	}
	source := strings.TrimSpace(upstream.Source)
	if source == "" {
		source = ModelAvailabilitySourceOpenRouter
	}
	apiMode := strings.TrimSpace(upstream.APIMode)
	if apiMode == "" {
		apiMode = APIModeRealtime
	}
	var existing ModelAvailability
	err := useDB.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_id = ?", modelID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status := ConnectivityUntested
		successes := 0
		visibleReason := ""
		if !ModelAutoVisibilityEnabled() && supportStatusForAPIMode(apiMode) == SupportSupported && apiMode == APIModeRealtime {
			status = ConnectivityConnected
			successes = 2
			visibleReason = VisibilityVisible
		}
		row := ModelAvailability{
			ModelID:              modelID,
			Source:               source,
			AdminEnabled:         true,
			ConnectivityStatus:   status,
			SDKMAXSupportStatus:  supportStatusForAPIMode(apiMode),
			APIMode:              apiMode,
			Capabilities:         capabilitiesToText(upstream.Capabilities),
			LastUpstreamSeenAt:   now,
			ConsecutiveFailures:  0,
			ConsecutiveSuccesses: successes,
			CreatedTime:          now,
			UpdatedTime:          now,
		}
		row.CustomerVisible, row.VisibilityReason = ResolveModelVisibility(row)
		if visibleReason != "" {
			row.VisibilityReason = visibleReason
		}
		return useDB.Create(&row).Error
	}
	if err != nil {
		return err
	}
	previous := existing
	existing.Source = source
	existing.APIMode = apiMode
	existing.Capabilities = capabilitiesToText(upstream.Capabilities)
	existing.LastUpstreamSeenAt = now
	existing.SDKMAXSupportStatus = supportStatusForAPIMode(apiMode)
	if existing.ConnectivityStatus == ConnectivityUpstreamMissing {
		existing.ConnectivityStatus = ConnectivityUntested
		existing.ConsecutiveFailures = 0
		existing.ConsecutiveRealFailures = 0
		existing.ConsecutiveSuccesses = 0
	}
	existing.CustomerVisible, existing.VisibilityReason = ResolveModelVisibility(existing)
	if !ModelAutoVisibilityEnabled() {
		existing.CustomerVisible = previous.CustomerVisible
		existing.VisibilityReason = previous.VisibilityReason
	}
	existing.UpdatedTime = now
	if err := useDB.Save(&existing).Error; err != nil {
		return err
	}
	if previous.CustomerVisible != existing.CustomerVisible || previous.ConnectivityStatus != existing.ConnectivityStatus {
		return createModelAvailabilityAuditLog(useDB, previous, existing, "openrouter_sync", "upstream catalog sync")
	}
	return nil
}

func MarkAvailabilityUpstreamMissing(tx *gorm.DB, modelIDs []string, now int64) error {
	if len(modelIDs) == 0 {
		return nil
	}
	if !ModelAutoVisibilityEnabled() {
		return nil
	}
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	if now == 0 {
		now = common.GetTimestamp()
	}
	return useDB.Model(&ModelAvailability{}).
		Where("model_id IN ?", modelIDs).
		Updates(map[string]interface{}{
			"connectivity_status": ConnectivityUpstreamMissing,
			"customer_visible":    false,
			"visibility_reason":   VisibilityUpstreamModelMissing,
			"last_failure_at":     now,
			"last_failure_reason": ConnectivityUpstreamMissing,
			"updated_time":        now,
		}).Error
}

func ApplyModelHealthCheckResult(tx *gorm.DB, modelID string, result ModelHealthCheckResult) (*ModelAvailability, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errors.New("model_id is required")
	}
	useDB := DB
	if tx != nil {
		useDB = tx
	} else {
		var row *ModelAvailability
		err := DB.Transaction(func(tx *gorm.DB) error {
			var txErr error
			row, txErr = ApplyModelHealthCheckResult(tx, modelID, result)
			return txErr
		})
		return row, err
	}
	var current ModelAvailability
	err := useDB.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_id = ?", modelID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := result.TestedAt
		if now == 0 {
			now = common.GetTimestamp()
		}
		current = ModelAvailability{
			ModelID:             modelID,
			Source:              ModelAvailabilitySourceOpenRouter,
			AdminEnabled:        true,
			ConnectivityStatus:  ConnectivityUntested,
			SDKMAXSupportStatus: SupportSupported,
			APIMode:             APIModeRealtime,
			CreatedTime:         now,
		}
	} else if err != nil {
		return nil, err
	}
	previous := current
	next := ApplyHealthResultToAvailability(current, result)
	if !ModelAutoVisibilityEnabled() {
		next.CustomerVisible = previous.CustomerVisible
		next.VisibilityReason = previous.VisibilityReason
	}
	if next.CreatedTime == 0 {
		next.CreatedTime = next.UpdatedTime
	}
	if err := useDB.Save(&next).Error; err != nil {
		return nil, err
	}
	if previous.CustomerVisible != next.CustomerVisible || previous.ConnectivityStatus != next.ConnectivityStatus {
		_ = createModelAvailabilityAuditLog(useDB, previous, next, "health_check", next.VisibilityReason)
	}
	return &next, nil
}

func SetModelAvailabilityAdminEnabled(modelID string, enabled bool, operatorID int, operatorName string) (*ModelAvailability, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errors.New("model_id is required")
	}
	var updated *ModelAvailability
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		var current ModelAvailability
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_id = ?", modelID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			current = ModelAvailability{
				ModelID:             modelID,
				Source:              ModelAvailabilitySourceOpenRouter,
				ConnectivityStatus:  ConnectivityUnknown,
				SDKMAXSupportStatus: SupportUnverified,
				APIMode:             APIModeRealtime,
				CreatedTime:         now,
			}
		} else if err != nil {
			return err
		}
		previous := current
		current.AdminEnabled = enabled
		current.CustomerVisible, current.VisibilityReason = ResolveModelVisibility(current)
		current.UpdatedTime = now
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		_ = createModelAvailabilityAuditLogWithOperator(tx, previous, current, "admin_update", current.VisibilityReason, operatorID, operatorName)
		updated = &current
		return nil
	})
	if err != nil {
		return nil, err
	}
	InvalidatePricingCache()
	InitChannelCache()
	return updated, nil
}

func MarkModelAvailabilityRetestRequested(modelID string, operatorID int, operatorName string) (*ModelAvailability, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errors.New("model_id is required")
	}
	now := common.GetTimestamp()
	var updated *ModelAvailability
	err := DB.Transaction(func(tx *gorm.DB) error {
		var current ModelAvailability
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("model_id = ?", modelID).First(&current).Error; err != nil {
			return err
		}
		previous := current
		current.LastRetestRequestedAt = now
		current.LastRetestRequestedByID = operatorID
		current.LastRetestRequestedByName = strings.TrimSpace(operatorName)
		if current.VisibilityReason == "" || current.VisibilityReason == VisibilityVisible {
			current.VisibilityReason = VisibilityRetestQueued
		}
		current.UpdatedTime = now
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		_ = createModelAvailabilityAuditLogWithOperator(tx, previous, current, "retest_requested", current.VisibilityReason, operatorID, operatorName)
		updated = &current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func GetModelAvailabilityMap() (map[string]ModelAvailability, error) {
	var rows []ModelAvailability
	if err := DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]ModelAvailability, len(rows))
	for _, row := range rows {
		result[row.ModelID] = row
	}
	return result, nil
}

func ModelIsCustomerVisible(modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}
	var row ModelAvailability
	err := DB.Select("customer_visible").Where("model_id = ?", modelID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if err != nil {
		common.SysLog("model availability lookup failed: " + err.Error())
		return true
	}
	return row.CustomerVisible
}

func createModelAvailabilityAuditLog(db *gorm.DB, previous, next ModelAvailability, action, detail string) error {
	return createModelAvailabilityAuditLogWithOperator(db, previous, next, action, detail, 0, "")
}

func createModelAvailabilityAuditLogWithOperator(db *gorm.DB, previous, next ModelAvailability, action, detail string, operatorID int, operatorName string) error {
	return db.Create(&ModelAvailabilityAuditLog{
		ModelID:         next.ModelID,
		Action:          action,
		PreviousVisible: previous.CustomerVisible,
		NextVisible:     next.CustomerVisible,
		PreviousStatus:  previous.ConnectivityStatus,
		NextStatus:      next.ConnectivityStatus,
		Reason:          next.VisibilityReason,
		Detail:          detail,
		OperatorID:      operatorID,
		OperatorName:    strings.TrimSpace(operatorName),
		CreatedTime:     common.GetTimestamp(),
	}).Error
}

func BootstrapModelAvailabilityFromAuditCSV(path string) (int, error) {
	explicitPath := strings.TrimSpace(path) != ""
	envPath := strings.TrimSpace(os.Getenv("MODEL_AVAILABILITY_AUDIT_CSV_PATH"))
	if !explicitPath {
		path = defaultModelAvailabilityAuditCSVPath()
		explicitPath = envPath != ""
	}
	records, err := readModelAvailabilityAuditCSVRecords(path, !explicitPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return bootstrapModelAvailabilityFromAuditCSVRecords(records)
}

func readModelAvailabilityAuditCSVRecords(path string, allowEmbeddedFallback bool) ([][]string, error) {
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		return csv.NewReader(file).ReadAll()
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if allowEmbeddedFallback && len(embeddedModelAvailabilityAuditCSV) > 0 {
		return csv.NewReader(bytes.NewReader(embeddedModelAvailabilityAuditCSV)).ReadAll()
	}
	return nil, err
}

func bootstrapModelAvailabilityFromAuditCSVRecords(records [][]string) (int, error) {
	if len(records) < 2 {
		return 0, nil
	}
	header := map[string]int{}
	for i, value := range records[0] {
		header[normalizeCSVHeader(value)] = i
	}
	get := func(record []string, key string) string {
		idx, ok := header[normalizeCSVHeader(key)]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}
	now := common.GetTimestamp()
	changed := 0
	for _, record := range records[1:] {
		modelID := get(record, "model")
		if modelID == "" {
			modelID = get(record, "model_id")
		}
		classification := strings.ToUpper(get(record, "classification"))
		if classification == "" {
			classification = strings.ToUpper(get(record, "primary_failure_reason"))
		}
		if modelID == "" || classification == "" {
			continue
		}
		var existing ModelAvailability
		err := DB.Where("model_id = ?", modelID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return changed, fmt.Errorf("bootstrap %s: %w", modelID, err)
		}
		if err == nil && existing.Source == ModelAvailabilitySourceAudit && existing.LastSuccessAt > existing.LastFailureAt {
			continue
		}
		adminEnabled := true
		createdTime := now
		if err == nil {
			adminEnabled = existing.AdminEnabled
			createdTime = existing.CreatedTime
			if createdTime == 0 {
				createdTime = now
			}
		}
		apiMode := apiModeFromAuditRecord(classification, get(record, "api_mode"), get(record, "is_batch"))
		support := supportFromAuditRecord(classification, apiMode)
		capabilities := strings.TrimSpace(get(record, "capabilities"))
		if capabilities == "" {
			capabilities = capabilitiesFromAuditClassification(classification)
		}
		failures, realFailures := auditFailureCounters(classification)
		row := ModelAvailability{
			ModelID:                 modelID,
			Source:                  ModelAvailabilitySourceAudit,
			AdminEnabled:            adminEnabled,
			ConnectivityStatus:      connectivityFromAuditClassification(classification),
			SDKMAXSupportStatus:     support,
			APIMode:                 apiMode,
			Capabilities:            capabilities,
			VisibilityReason:        visibilityFromAuditClassification(classification),
			ConsecutiveFailures:     failures,
			ConsecutiveRealFailures: realFailures,
			LastTestedAt:            now,
			LastFailureAt:           now,
			LastFailureReason:       classification,
			LastError:               firstNonEmpty(get(record, "error"), get(record, "original_error")),
			LastHTTPStatus:          common.String2Int(get(record, "http_status")),
			CreatedTime:             createdTime,
			UpdatedTime:             now,
		}
		row.CustomerVisible, row.VisibilityReason = ResolveModelVisibility(row)
		if err == nil {
			row.Id = existing.Id
			if err := DB.Save(&row).Error; err != nil {
				return changed, fmt.Errorf("bootstrap %s: %w", modelID, err)
			}
		} else if err := DB.Create(&row).Error; err != nil {
			return changed, fmt.Errorf("bootstrap %s: %w", modelID, err)
		}
		changed++
	}
	if changed > 0 {
		InvalidatePricingCache()
		InitChannelCache()
	}
	return changed, nil
}

func BackfillModelAvailabilityFromEnabledAbilities() (int, error) {
	var modelIDs []string
	if err := DB.Model(&Ability{}).Where("enabled = ?", true).Distinct("model").Pluck("model", &modelIDs).Error; err != nil {
		return 0, err
	}
	now := common.GetTimestamp()
	created := 0
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		var existing ModelAvailability
		err := DB.Select("id").Where("model_id = ?", modelID).First(&existing).Error
		if err == nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return created, err
		}
		apiMode := apiModeFromModelID(modelID)
		row := ModelAvailability{
			ModelID:              modelID,
			Source:               ModelAvailabilitySourceBackfill,
			AdminEnabled:         true,
			ConnectivityStatus:   ConnectivityConnected,
			SDKMAXSupportStatus:  supportStatusForAPIMode(apiMode),
			APIMode:              apiMode,
			Capabilities:         capabilityForAPIMode(apiMode),
			ConsecutiveSuccesses: 2,
			LastTestedAt:         now,
			LastSuccessAt:        now,
			CreatedTime:          now,
			UpdatedTime:          now,
		}
		row.CustomerVisible, row.VisibilityReason = ResolveModelVisibility(row)
		if err := DB.Create(&row).Error; err != nil {
			return created, err
		}
		created++
	}
	if created > 0 {
		InvalidatePricingCache()
		InitChannelCache()
	}
	return created, nil
}

func defaultModelAvailabilityAuditCSVPath() string {
	if path := strings.TrimSpace(os.Getenv("MODEL_AVAILABILITY_AUDIT_CSV_PATH")); path != "" {
		return path
	}
	candidates := []string{
		filepath.Join("D:"+string(os.PathSeparator), "sdkmax", "docs", "AUDIT", "model-failure-reclassification-202608.csv"),
		filepath.Join("D:"+string(os.PathSeparator), "sdkmax", "docs", "audit", "model-failure-reclassification-202608.csv"),
		filepath.Join("docs", "AUDIT", "model-failure-reclassification-202608.csv"),
		filepath.Join("docs", "audit", "model-failure-reclassification-202608.csv"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func normalizeCSVHeader(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.TrimSpace(strings.ToLower(value))
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func apiModeFromAuditRecord(classification, rawMode, rawBatch string) string {
	rawMode = strings.TrimSpace(strings.ToLower(rawMode))
	if rawMode != "" {
		switch rawMode {
		case APIModeRealtime, APIModeBatch, APIModeAsync, APIModeImage, APIModeVideo, APIModeAudio, APIModeEmbedding, APIModeSearch:
			return rawMode
		default:
			if strings.Contains(rawMode, "batch") {
				return APIModeBatch
			}
			if strings.Contains(rawMode, "image") {
				return APIModeImage
			}
			if strings.Contains(rawMode, "video") {
				return APIModeVideo
			}
			if strings.Contains(rawMode, "audio") || strings.Contains(rawMode, "special") {
				return APIModeAudio
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(rawBatch), "true") {
		return APIModeBatch
	}
	return apiModeFromAuditClassification(classification)
}

func supportFromAuditRecord(classification, apiMode string) string {
	if apiMode != APIModeRealtime && apiMode != APIModeEmbedding {
		if apiMode == APIModeBatch {
			return SupportUnsupported
		}
		return SupportUnverified
	}
	return supportFromAuditClassification(classification)
}

func auditFailureCounters(classification string) (int, int) {
	switch classification {
	case "NO_PROVIDER", "RATE_LIMITED", "WRONG_ENDPOINT", "WRONG_PAYLOAD", "BATCH_API_REQUIRED", "UNSUPPORTED_BY_SDKMAX":
		return 1, 0
	default:
		return 3, 3
	}
}

func apiModeFromModelID(modelID string) string {
	lowerID := strings.ToLower(strings.TrimSpace(modelID))
	if strings.Contains(lowerID, ":batch") || strings.HasSuffix(lowerID, "-batch") {
		return APIModeBatch
	}
	if common.IsImageGenerationModel(modelID) {
		return APIModeImage
	}
	return APIModeRealtime
}

func capabilityForAPIMode(apiMode string) string {
	switch apiMode {
	case APIModeImage:
		return "image_generation"
	case APIModeVideo:
		return "video_generation"
	case APIModeAudio:
		return "audio"
	case APIModeEmbedding:
		return "embedding"
	case APIModeSearch:
		return "search"
	default:
		return "text"
	}
}

func connectivityFromAuditClassification(classification string) string {
	switch classification {
	case "BATCH_API_REQUIRED":
		return ConnectivityBatchRequired
	case "MODEL_NOT_FOUND":
		return ConnectivityUpstreamMissing
	case "WRONG_PAYLOAD":
		return ConnectivityWrongPayload
	case "WRONG_ENDPOINT":
		return ConnectivityWrongEndpoint
	case "UNSUPPORTED_BY_SDKMAX":
		return ConnectivityUnsupported
	case "NO_PROVIDER":
		return ConnectivityDegraded
	case "RATE_LIMITED":
		return ConnectivityDegraded
	case "TIMEOUT":
		return ConnectivityTimeout
	case "UPSTREAM_ERROR", "AUTH_ERROR":
		return ConnectivityUpstreamError
	default:
		return ConnectivityFailed
	}
}

func supportFromAuditClassification(classification string) string {
	switch classification {
	case "BATCH_API_REQUIRED", "UNSUPPORTED_BY_SDKMAX":
		return SupportUnsupported
	case "WRONG_PAYLOAD", "WRONG_ENDPOINT":
		return SupportUnverified
	default:
		return SupportSupported
	}
}

func apiModeFromAuditClassification(classification string) string {
	if classification == "BATCH_API_REQUIRED" {
		return APIModeBatch
	}
	return APIModeRealtime
}

func visibilityFromAuditClassification(classification string) string {
	switch classification {
	case "BATCH_API_REQUIRED":
		return VisibilityBatchNotSupported
	case "MODEL_NOT_FOUND":
		return VisibilityUpstreamModelMissing
	case "WRONG_PAYLOAD":
		return VisibilityWrongPayload
	case "WRONG_ENDPOINT":
		return VisibilityWrongEndpoint
	case "UNSUPPORTED_BY_SDKMAX":
		return VisibilitySdkmaxUnsupported
	case "NO_PROVIDER":
		return VisibilityNoProviderRetest
	case "RATE_LIMITED":
		return VisibilityRateLimitedRetest
	default:
		return VisibilityConnectivityFailed
	}
}

func capabilitiesFromAuditClassification(classification string) string {
	if classification == "BATCH_API_REQUIRED" {
		return "text"
	}
	return "text"
}
