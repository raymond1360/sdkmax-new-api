package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	minOpenRouterMultiplier = 0.01
	maxOpenRouterMultiplier = 10.00
)

type openRouterMultiplierRequest struct {
	Multiplier string `json:"multiplier"`
}

type openRouterModelMultiplierRequest struct {
	ModelID    string `json:"model_id"`
	Multiplier string `json:"multiplier"`
}

func normalizeAllowedMultiplier(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "1.00"
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < minOpenRouterMultiplier || parsed > maxOpenRouterMultiplier {
		return "", false
	}
	return model.FormatOpenRouterMultiplier(parsed), true
}

func recalculateOpenRouterSalePrices(modelID string) error {
	globalMultiplier := model.GetOpenRouterGlobalMultiplier()
	multipliers, err := model.GetOpenRouterModelMultipliers()
	if err != nil {
		return err
	}
	query := model.DB.Model(&model.OpenRouterModel{})
	if strings.TrimSpace(modelID) != "" {
		query = query.Where("model_id = ?", modelID)
	}
	var rows []model.OpenRouterModel
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	modelIDs := make([]string, 0, len(rows))
	optionUpdates, err := buildOpenRouterBillingOptionUpdates(rows, globalMultiplier, multipliers)
	if err != nil {
		return err
	}
	for _, row := range rows {
		modelIDs = append(modelIDs, row.ModelID)
	}
	cleanupUpdates, err := service.RemoveOpenRouterManualPricingOptions(modelIDs)
	if err != nil {
		return err
	}
	for key, value := range cleanupUpdates {
		optionUpdates[key] = value
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			modelMultiplier := model.OpenRouterDefaultModelMultiplier
			modelMultiplierRaw := model.FormatOpenRouterMultiplier(modelMultiplier)
			if raw := multipliers[row.ModelID]; raw != "" {
				parsed, ok := normalizeAllowedMultiplier(raw)
				if ok {
					modelMultiplierRaw = parsed
					modelMultiplier, _ = strconv.ParseFloat(parsed, 64)
				}
			}
			row.FillPriceFieldsFromLegacy(globalMultiplier, modelMultiplier)
			sdkmaxInputPrice := model.CalculateOpenRouterSalePrice(row.OpenRouterInputPrice, globalMultiplier, modelMultiplier)
			sdkmaxOutputPrice := model.CalculateOpenRouterSalePrice(row.OpenRouterOutputPrice, globalMultiplier, modelMultiplier)
			updates := map[string]interface{}{
				"open_router_input_price":  row.OpenRouterInputPrice,
				"open_router_output_price": row.OpenRouterOutputPrice,
				"sdkmax_input_price":       sdkmaxInputPrice,
				"sdkmax_output_price":      sdkmaxOutputPrice,
				"global_multiplier":        model.FormatOpenRouterMultiplier(globalMultiplier),
				"model_multiplier":         modelMultiplierRaw,
				"updated_time":             now,
			}
			if err := tx.Model(&model.OpenRouterModel{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return model.UpdateOptionsBulkInTx(tx, optionUpdates)
	}); err != nil {
		return err
	}
	if err := model.ApplyOptionMapUpdates(optionUpdates); err != nil {
		return err
	}
	model.InvalidatePricingCache()
	return nil
}

func buildOpenRouterBillingOptionUpdates(rows []model.OpenRouterModel, globalMultiplier float64, multipliers map[string]string) (map[string]string, error) {
	billingModes := billing_setting.GetBillingModeCopy()
	billingExprs := billing_setting.GetBillingExprCopy()
	for _, row := range rows {
		modelMultiplier := model.OpenRouterDefaultModelMultiplier
		if raw := multipliers[row.ModelID]; raw != "" {
			if parsed, ok := normalizeAllowedMultiplier(raw); ok {
				modelMultiplier, _ = strconv.ParseFloat(parsed, 64)
			}
		}
		row.FillPriceFieldsFromLegacy(globalMultiplier, modelMultiplier)
		sdkmaxInputPrice := model.CalculateOpenRouterSalePrice(row.OpenRouterInputPrice, globalMultiplier, modelMultiplier)
		sdkmaxOutputPrice := model.CalculateOpenRouterSalePrice(row.OpenRouterOutputPrice, globalMultiplier, modelMultiplier)
		billingModes[row.ModelID] = billing_setting.BillingModeTieredExpr
		billingExprs[row.ModelID] = buildOpenRouterBillingExpr(sdkmaxInputPrice, sdkmaxOutputPrice)
	}
	modeBytes, err := common.Marshal(billingModes)
	if err != nil {
		return nil, err
	}
	exprBytes, err := common.Marshal(billingExprs)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"billing_setting.billing_mode": string(modeBytes),
		"billing_setting.billing_expr": string(exprBytes),
	}, nil
}

func buildOpenRouterBillingExpr(inputPricePerToken, outputPricePerToken string) string {
	inputPrice := pricePerTokenToPerMillion(inputPricePerToken)
	outputPrice := pricePerTokenToPerMillion(outputPricePerToken)
	return fmt.Sprintf("tier(\"openrouter\", p * %s + c * %s)", inputPrice, outputPrice)
}

func pricePerTokenToPerMillion(value string) string {
	price, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || price <= 0 {
		return "0"
	}
	return strconv.FormatFloat(price*1000000, 'f', 10, 64)
}

func GetOpenRouterSyncState(c *gin.Context) {
	limit := common.String2Int(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var models []model.OpenRouterModel
	if err := model.DB.Order("model_id ASC").Limit(limit).Find(&models).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	logs, _ := model.GetOpenRouterSyncLogs(20)
	multipliers, _ := model.GetOpenRouterModelMultipliers()
	globalMultiplier := model.GetOpenRouterGlobalMultiplier()
	for idx := range models {
		modelMultiplier := model.OpenRouterDefaultModelMultiplier
		if raw := multipliers[models[idx].ModelID]; raw != "" {
			if parsed, ok := normalizeAllowedMultiplier(raw); ok {
				modelMultiplier, _ = strconv.ParseFloat(parsed, 64)
			}
		}
		models[idx].FillPriceFieldsFromLegacy(globalMultiplier, modelMultiplier)
	}
	common.OptionMapRWMutex.RLock()
	settings := gin.H{
		"global_multiplier":   common.OptionMap["OpenRouterGlobalPriceMultiplier"],
		"unified_channel_id":  common.OptionMap["OpenRouterUnifiedChannelId"],
		"auto_sync_enabled":   common.OptionMap["OpenRouterAutoSyncEnabled"] == "true",
		"interval_minutes":    common.OptionMap["OpenRouterAutoSyncIntervalMinutes"],
		"allowed_multipliers": []string{"0.90", "1.00", "1.05", "1.10", "1.30"},
	}
	common.OptionMapRWMutex.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"settings":          settings,
			"models":            models,
			"model_multipliers": multipliers,
			"logs":              logs,
		},
	})
}

func TriggerOpenRouterSync(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	result, err := service.SyncOpenRouterModels(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func UpdateOpenRouterGlobalMultiplier(c *gin.Context) {
	var req openRouterMultiplierRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	multiplier, ok := normalizeAllowedMultiplier(req.Multiplier)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid multiplier, expected 0.01 - 10.00"})
		return
	}
	if err := model.UpdateOption("OpenRouterGlobalPriceMultiplier", multiplier); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := recalculateOpenRouterSalePrices(""); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func UpdateOpenRouterModelMultiplier(c *gin.Context) {
	var req openRouterModelMultiplierRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	req.ModelID = strings.TrimSpace(req.ModelID)
	if req.ModelID == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "model_id is required"})
		return
	}
	multiplier, ok := normalizeAllowedMultiplier(req.Multiplier)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid multiplier, expected 0.01 - 10.00"})
		return
	}
	if multiplier == "1.00" {
		if err := model.DeleteOpenRouterModelMultiplier(req.ModelID); err != nil {
			common.ApiError(c, err)
			return
		}
	} else if err := model.UpsertOpenRouterModelMultiplier(req.ModelID, multiplier); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := recalculateOpenRouterSalePrices(req.ModelID); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func GetOpenRouterChannels(c *gin.Context) {
	var channels []model.Channel
	if err := model.DB.Where("type = ?", constant.ChannelTypeOpenRouter).Order("id ASC").Omit("key").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(channels))
	for _, channel := range channels {
		items = append(items, gin.H{
			"id":       channel.Id,
			"name":     channel.Name,
			"status":   channel.Status,
			"base_url": channel.GetBaseURL(),
			"models":   len(channel.GetModels()),
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

func UpdateOpenRouterUnifiedChannel(c *gin.Context) {
	var req struct {
		ChannelID int `json:"channel_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	if req.ChannelID > 0 {
		var count int64
		if err := model.DB.Model(&model.Channel{}).Where("id = ? AND type = ?", req.ChannelID, constant.ChannelTypeOpenRouter).Count(&count).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if count == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": fmt.Sprintf("OpenRouter channel %d not found", req.ChannelID)})
			return
		}
	}
	if err := model.UpdateOption("OpenRouterUnifiedChannelId", strconv.Itoa(req.ChannelID)); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
