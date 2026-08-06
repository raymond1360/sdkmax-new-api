package model

import (
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

const (
	OpenRouterDefaultGlobalMultiplier = 1.00
	OpenRouterDefaultModelMultiplier  = 1.00

	OpenRouterSyncStatusRunning = "running"
	OpenRouterSyncStatusSuccess = "success"
	OpenRouterSyncStatusFailed  = "failed"
)

type OpenRouterModel struct {
	Id                         int    `json:"id"`
	ModelID                    string `json:"model_id" gorm:"size:255;not null;uniqueIndex"`
	ModelName                  string `json:"model_name" gorm:"size:255;not null"`
	Provider                   string `json:"provider" gorm:"size:128;index"`
	ContextLength              int64  `json:"context_length" gorm:"bigint;default:0"`
	PromptPrice                string `json:"-" gorm:"column:prompt_price;->"`
	CompletionPrice            string `json:"-" gorm:"column:completion_price;->"`
	InputPrice                 string `json:"-" gorm:"column:input_price;->"`
	OutputPrice                string `json:"-" gorm:"column:output_price;->"`
	OpenRouterInputPrice       string `json:"openrouter_input_price" gorm:"type:varchar(64);not null;default:'0'"`
	OpenRouterOutputPrice      string `json:"openrouter_output_price" gorm:"type:varchar(64);not null;default:'0'"`
	SDKMAXInputPrice           string `json:"sdkmax_input_price" gorm:"type:varchar(64);not null;default:'0'"`
	SDKMAXOutputPrice          string `json:"sdkmax_output_price" gorm:"type:varchar(64);not null;default:'0'"`
	GlobalMultiplier           string `json:"global_multiplier" gorm:"type:varchar(16);not null;default:'1.00'"`
	ModelMultiplier            string `json:"model_multiplier" gorm:"type:varchar(16);not null;default:'1.00'"`
	UnifiedOpenRouterChannelID int    `json:"unified_openrouter_channel_id" gorm:"index;default:0"`
	Healthy                    bool   `json:"healthy" gorm:"default:true;index"`
	HealthMessage              string `json:"health_message" gorm:"type:varchar(255);default:''"`
	LastSyncedAt               int64  `json:"last_synced_at" gorm:"bigint"`
	LastHealthCheckedAt        int64  `json:"last_health_checked_at" gorm:"bigint"`
	CreatedTime                int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime                int64  `json:"updated_time" gorm:"bigint"`
}

type OpenRouterModelMultiplier struct {
	ModelID     string `json:"model_id" gorm:"size:255;primaryKey;autoIncrement:false"`
	Multiplier  string `json:"multiplier" gorm:"type:varchar(16);not null;default:'1.00'"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

type OpenRouterSyncLog struct {
	Id             int    `json:"id"`
	Status         string `json:"status" gorm:"type:varchar(32);index"`
	Message        string `json:"message" gorm:"type:text"`
	ModelsFetched  int    `json:"models_fetched"`
	ModelsCreated  int    `json:"models_created"`
	ModelsUpdated  int    `json:"models_updated"`
	ModelsDisabled int    `json:"models_disabled"`
	ChannelID      int    `json:"channel_id" gorm:"index;default:0"`
	StartedAt      int64  `json:"started_at" gorm:"bigint;index"`
	FinishedAt     int64  `json:"finished_at" gorm:"bigint;index"`
	DurationMs     int64  `json:"duration_ms" gorm:"bigint;default:0"`
}

func parseOpenRouterMultiplier(value string, fallback float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func FormatOpenRouterMultiplier(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func GetOpenRouterGlobalMultiplier() float64 {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["OpenRouterGlobalPriceMultiplier"]
	common.OptionMapRWMutex.RUnlock()
	return parseOpenRouterMultiplier(raw, OpenRouterDefaultGlobalMultiplier)
}

func CalculateOpenRouterSalePrice(rawPrice string, globalMultiplier, modelMultiplier float64) string {
	price, err := strconv.ParseFloat(strings.TrimSpace(rawPrice), 64)
	if err != nil || price <= 0 {
		return "0"
	}
	result := price * globalMultiplier * modelMultiplier
	if result == 0 {
		return "0"
	}
	if math.Abs(result) < 0.000000001 {
		return strconv.FormatFloat(result, 'f', 12, 64)
	}
	return strconv.FormatFloat(result, 'f', 10, 64)
}

func NormalizeOpenRouterPrice(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	price, err := strconv.ParseFloat(value, 64)
	if err != nil || price <= 0 {
		return "0"
	}
	return strconv.FormatFloat(price, 'f', 12, 64)
}

func IsZeroOpenRouterPrice(value string) bool {
	return NormalizeOpenRouterPrice(value) == "0"
}

func (m *OpenRouterModel) FillPriceFieldsFromLegacy(globalMultiplier float64, modelMultiplier float64) {
	if IsZeroOpenRouterPrice(m.OpenRouterInputPrice) && !IsZeroOpenRouterPrice(m.PromptPrice) {
		m.OpenRouterInputPrice = NormalizeOpenRouterPrice(m.PromptPrice)
	}
	if IsZeroOpenRouterPrice(m.OpenRouterOutputPrice) && !IsZeroOpenRouterPrice(m.CompletionPrice) {
		m.OpenRouterOutputPrice = NormalizeOpenRouterPrice(m.CompletionPrice)
	}
	if !IsZeroOpenRouterPrice(m.OpenRouterInputPrice) {
		m.OpenRouterInputPrice = NormalizeOpenRouterPrice(m.OpenRouterInputPrice)
	}
	if !IsZeroOpenRouterPrice(m.OpenRouterOutputPrice) {
		m.OpenRouterOutputPrice = NormalizeOpenRouterPrice(m.OpenRouterOutputPrice)
	}
	m.SDKMAXInputPrice = CalculateOpenRouterSalePrice(m.OpenRouterInputPrice, globalMultiplier, modelMultiplier)
	m.SDKMAXOutputPrice = CalculateOpenRouterSalePrice(m.OpenRouterOutputPrice, globalMultiplier, modelMultiplier)
}

func GetOpenRouterModelMultipliers() (map[string]string, error) {
	var rows []OpenRouterModelMultiplier
	if err := DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.ModelID] = row.Multiplier
	}
	return result, nil
}

func UpsertOpenRouterModelMultiplier(modelID, multiplier string) error {
	now := common.GetTimestamp()
	row := OpenRouterModelMultiplier{
		ModelID:     modelID,
		Multiplier:  multiplier,
		CreatedTime: now,
		UpdatedTime: now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"multiplier":   multiplier,
			"updated_time": now,
		}),
	}).Create(&row).Error
}

func DeleteOpenRouterModelMultiplier(modelID string) error {
	return DB.Delete(&OpenRouterModelMultiplier{ModelID: modelID}).Error
}

func CreateOpenRouterSyncLog(status, message string) (*OpenRouterSyncLog, error) {
	now := common.GetTimestamp()
	log := &OpenRouterSyncLog{
		Status:    status,
		Message:   message,
		StartedAt: now,
	}
	return log, DB.Create(log).Error
}

func FinishOpenRouterSyncLog(log *OpenRouterSyncLog, status, message string, fetched, created, updated, disabled, channelID int) error {
	if log == nil {
		return nil
	}
	now := common.GetTimestamp()
	log.Status = status
	log.Message = message
	log.ModelsFetched = fetched
	log.ModelsCreated = created
	log.ModelsUpdated = updated
	log.ModelsDisabled = disabled
	log.ChannelID = channelID
	log.FinishedAt = now
	log.DurationMs = (now - log.StartedAt) * 1000
	return DB.Save(log).Error
}

func GetOpenRouterSyncLogs(limit int) ([]OpenRouterSyncLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var logs []OpenRouterSyncLog
	err := DB.Order("id DESC").Limit(limit).Find(&logs).Error
	return logs, err
}
