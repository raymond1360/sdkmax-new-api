package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"gorm.io/gorm"
)

const openRouterModelsURL = "https://openrouter.ai/api/v1/models"
const (
	openRouterFetchDropGuardPercent = 80
	openRouterFetchBaselineMin      = 100
)

var openRouterSyncLock sync.Mutex

type OpenRouterSyncResult struct {
	ModelsFetched  int `json:"models_fetched"`
	ModelsCreated  int `json:"models_created"`
	ModelsUpdated  int `json:"models_updated"`
	ModelsDisabled int `json:"models_disabled"`
	ChannelID      int `json:"channel_id"`
}

type openRouterModelsResponse struct {
	Data []openRouterAPIModel `json:"data"`
}

type openRouterAPIModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	ContextLength int64                  `json:"context_length"`
	Pricing       openRouterModelPricing `json:"pricing"`
	TopProvider   openRouterTopProvider  `json:"top_provider"`
	Architecture  openRouterArchitecture `json:"architecture"`
}

type openRouterModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type openRouterTopProvider struct {
	ContextLength int64 `json:"context_length"`
}

type openRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

func FetchOpenRouterModels(ctx context.Context) ([]openRouterAPIModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client, err := NewProxyHttpClient("")
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter models returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	var parsed openRouterModelsResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Data, nil
}

func providerFromOpenRouterModelID(modelID string) string {
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	provider := strings.TrimPrefix(strings.TrimSpace(parts[0]), "~")
	switch strings.ToLower(provider) {
	case "anthropic":
		return "Anthropic"
	case "google":
		return "Google"
	case "moonshotai":
		return "Moonshot"
	case "openai":
		return "OpenAI"
	case "x-ai":
		return "xAI"
	case "deepseek":
		return "DeepSeek"
	}
	return provider
}

func deriveOpenRouterAvailability(upstream openRouterAPIModel, seenAt int64) model.ModelAvailabilityUpstream {
	modelID := strings.TrimSpace(upstream.ID)
	capabilities := []string{}
	for _, modality := range upstream.Architecture.InputModalities {
		modality = strings.ToLower(strings.TrimSpace(modality))
		switch modality {
		case "text":
			capabilities = append(capabilities, "text")
		case "image":
			capabilities = append(capabilities, "vision")
		case "audio":
			capabilities = append(capabilities, "audio")
		}
	}
	for _, modality := range upstream.Architecture.OutputModalities {
		modality = strings.ToLower(strings.TrimSpace(modality))
		switch modality {
		case "text":
			capabilities = append(capabilities, "text")
		case "image":
			capabilities = append(capabilities, "image_generation")
		case "video":
			capabilities = append(capabilities, "video_generation")
		case "audio":
			capabilities = append(capabilities, "audio")
		}
	}
	if len(capabilities) == 0 {
		capabilities = append(capabilities, "text")
	}
	apiMode := model.APIModeRealtime
	lowerID := strings.ToLower(modelID)
	if strings.Contains(lowerID, ":batch") || strings.HasSuffix(lowerID, "-batch") {
		apiMode = model.APIModeBatch
	} else if containsStringFold(upstream.Architecture.OutputModalities, "video") {
		apiMode = model.APIModeVideo
	} else if containsStringFold(upstream.Architecture.OutputModalities, "image") {
		apiMode = model.APIModeImage
	} else if containsStringFold(upstream.Architecture.OutputModalities, "audio") {
		apiMode = model.APIModeAudio
	}
	return model.ModelAvailabilityUpstream{
		ModelID:      modelID,
		Source:       model.ModelAvailabilitySourceOpenRouter,
		APIMode:      apiMode,
		Capabilities: capabilities,
		SeenAt:       seenAt,
	}
}

func containsStringFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func uniqueOpenRouterModelIDs(ids ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, group := range ids {
		for _, id := range group {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

func removeOpenRouterManualPricingFromOptionMap(optionKey string, modelIDs []string) (string, bool, error) {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[optionKey]
	common.OptionMapRWMutex.RUnlock()
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}

	values := make(map[string]interface{})
	if err := common.Unmarshal([]byte(raw), &values); err != nil {
		return "", false, err
	}

	changed := false
	for _, modelID := range modelIDs {
		if _, ok := values[modelID]; ok {
			delete(values, modelID)
			changed = true
		}
	}
	if !changed {
		return "", false, nil
	}

	bytes, err := common.Marshal(values)
	if err != nil {
		return "", false, err
	}
	return string(bytes), true, nil
}

func RemoveOpenRouterManualPricingOptions(modelIDs []string) (map[string]string, error) {
	modelIDs = uniqueOpenRouterModelIDs(modelIDs)
	if len(modelIDs) == 0 {
		return nil, nil
	}

	optionKeys := []string{
		"ModelPrice",
		"ModelRatio",
		"CompletionRatio",
		"CacheRatio",
		"CreateCacheRatio",
		"ImageRatio",
		"AudioRatio",
		"AudioCompletionRatio",
	}
	updates := make(map[string]string)
	for _, optionKey := range optionKeys {
		value, changed, err := removeOpenRouterManualPricingFromOptionMap(optionKey, modelIDs)
		if err != nil {
			return nil, fmt.Errorf("clean %s: %w", optionKey, err)
		}
		if changed {
			updates[optionKey] = value
		}
	}
	return updates, nil
}

func isUnsafeOpenRouterFetchCount(fetched, baseline int) bool {
	return baseline >= openRouterFetchBaselineMin && fetched*100 < baseline*openRouterFetchDropGuardPercent
}

func validateOpenRouterFetchSafety(fetched int) error {
	if fetched == 0 {
		return errors.New("OpenRouter models API returned 0 models; refusing to overwrite channel models or pricing")
	}

	var lastSuccess model.OpenRouterSyncLog
	err := model.DB.Where("status = ?", model.OpenRouterSyncStatusSuccess).
		Order("id DESC").
		First(&lastSuccess).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && isUnsafeOpenRouterFetchCount(fetched, lastSuccess.ModelsFetched) {
		return fmt.Errorf("OpenRouter models API returned %d models, below %d%% of last successful sync count %d; refusing to overwrite channel models or pricing", fetched, openRouterFetchDropGuardPercent, lastSuccess.ModelsFetched)
	}

	var existingCount int64
	if err := model.DB.Model(&model.OpenRouterModel{}).Count(&existingCount).Error; err != nil {
		return err
	}
	if isUnsafeOpenRouterFetchCount(fetched, int(existingCount)) {
		return fmt.Errorf("OpenRouter models API returned %d models, below %d%% of existing OpenRouter model count %d; refusing to overwrite channel models or pricing", fetched, openRouterFetchDropGuardPercent, existingCount)
	}
	return nil
}

func detectEnabledNonOpenRouterModelConflicts(openRouterModelIDs []string) ([]string, error) {
	modelSet := make(map[string]struct{}, len(openRouterModelIDs))
	for _, modelID := range openRouterModelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID != "" {
			modelSet[modelID] = struct{}{}
		}
	}
	if len(modelSet) == 0 {
		return nil, nil
	}

	var channels []model.Channel
	if err := model.DB.
		Where("status = ? AND type <> ?", common.ChannelStatusEnabled, constant.ChannelTypeOpenRouter).
		Find(&channels).Error; err != nil {
		return nil, err
	}

	conflicts := make([]string, 0)
	seen := make(map[string]struct{})
	for _, channel := range channels {
		for _, channelModel := range channel.GetModels() {
			channelModel = strings.TrimSpace(channelModel)
			if _, ok := modelSet[channelModel]; !ok {
				continue
			}
			key := fmt.Sprintf("%s#%d", channelModel, channel.Id)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			conflicts = append(conflicts, fmt.Sprintf("%s (channel #%d %s)", channelModel, channel.Id, channel.Name))
		}
	}
	sort.Strings(conflicts)
	return conflicts, nil
}

func getConfiguredOpenRouterChannelID() int {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["OpenRouterUnifiedChannelId"]
	common.OptionMapRWMutex.RUnlock()
	return common.String2Int(raw)
}

func FindUnifiedOpenRouterChannel(tx *gorm.DB) (*model.Channel, error) {
	configuredID := getConfiguredOpenRouterChannelID()
	if configuredID > 0 {
		var channel model.Channel
		if err := tx.Where("id = ? AND type = ?", configuredID, constant.ChannelTypeOpenRouter).First(&channel).Error; err != nil {
			return nil, err
		}
		return &channel, nil
	}

	var channel model.Channel
	err := tx.Where("type = ?", constant.ChannelTypeOpenRouter).
		Order("status ASC").
		Order("priority DESC").
		Order("id ASC").
		First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func SyncOpenRouterModels(ctx context.Context) (*OpenRouterSyncResult, error) {
	if !openRouterSyncLock.TryLock() {
		return nil, errors.New("openrouter sync is already running")
	}
	defer openRouterSyncLock.Unlock()

	syncLog, _ := model.CreateOpenRouterSyncLog(model.OpenRouterSyncStatusRunning, "openrouter sync started")
	result := &OpenRouterSyncResult{}
	finishLog := func(status string, err error) {
		message := "openrouter sync completed"
		if err != nil {
			message = err.Error()
		}
		_ = model.FinishOpenRouterSyncLog(syncLog, status, message, result.ModelsFetched, result.ModelsCreated, result.ModelsUpdated, result.ModelsDisabled, result.ChannelID)
	}

	upstreamModels, err := FetchOpenRouterModels(ctx)
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	upstreamModelIDs := make([]string, 0, len(upstreamModels))
	for _, upstream := range upstreamModels {
		if modelID := strings.TrimSpace(upstream.ID); modelID != "" {
			upstreamModelIDs = append(upstreamModelIDs, modelID)
		}
	}
	result.ModelsFetched = len(upstreamModelIDs)
	if err := validateOpenRouterFetchSafety(result.ModelsFetched); err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	conflicts, err := detectEnabledNonOpenRouterModelConflicts(upstreamModelIDs)
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	if len(conflicts) > 0 {
		preview := conflicts
		if len(preview) > 10 {
			preview = preview[:10]
		}
		err := fmt.Errorf("OpenRouter sync would overwrite global pricing for model IDs also exposed by enabled non-OpenRouter channels: %s", strings.Join(preview, "; "))
		if len(conflicts) > len(preview) {
			err = fmt.Errorf("%w; and %d more", err, len(conflicts)-len(preview))
		}
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}

	globalMultiplier := model.GetOpenRouterGlobalMultiplier()
	modelMultipliers, err := model.GetOpenRouterModelMultipliers()
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	billingModes := billing_setting.GetBillingModeCopy()
	billingExprs := billing_setting.GetBillingExprCopy()
	var cleanupModelIDs []string

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		channel, err := FindUnifiedOpenRouterChannel(tx)
		if err != nil {
			return fmt.Errorf("unified OpenRouter channel not found: %w", err)
		}
		result.ChannelID = channel.Id

		now := common.GetTimestamp()
		seen := make(map[string]struct{}, len(upstreamModels))
		modelIDs := make([]string, 0, len(upstreamModels))
		var existingOpenRouterModelIDs []string
		if err := tx.Model(&model.OpenRouterModel{}).Pluck("model_id", &existingOpenRouterModelIDs).Error; err != nil {
			return err
		}

		for _, upstream := range upstreamModels {
			modelID := strings.TrimSpace(upstream.ID)
			if modelID == "" {
				continue
			}
			seen[modelID] = struct{}{}
			modelIDs = append(modelIDs, modelID)
			contextLength := upstream.ContextLength
			if contextLength <= 0 {
				contextLength = upstream.TopProvider.ContextLength
			}
			modelMultiplier := model.OpenRouterDefaultModelMultiplier
			modelMultiplierRaw := model.FormatOpenRouterMultiplier(modelMultiplier)
			if raw := modelMultipliers[modelID]; raw != "" {
				modelMultiplier = parseMultiplier(raw, model.OpenRouterDefaultModelMultiplier)
				modelMultiplierRaw = model.FormatOpenRouterMultiplier(modelMultiplier)
			}
			globalMultiplierRaw := model.FormatOpenRouterMultiplier(globalMultiplier)
			sdkmaxInputPrice := model.CalculateOpenRouterSalePrice(upstream.Pricing.Prompt, globalMultiplier, modelMultiplier)
			sdkmaxOutputPrice := model.CalculateOpenRouterSalePrice(upstream.Pricing.Completion, globalMultiplier, modelMultiplier)
			billingModes[modelID] = billing_setting.BillingModeTieredExpr
			billingExprs[modelID] = buildOpenRouterBillingExpr(sdkmaxInputPrice, sdkmaxOutputPrice)

			var existing model.OpenRouterModel
			dbErr := tx.Where("model_id = ?", modelID).First(&existing).Error
			row := model.OpenRouterModel{
				ModelID:                    modelID,
				ModelName:                  strings.TrimSpace(upstream.Name),
				Provider:                   providerFromOpenRouterModelID(modelID),
				ContextLength:              contextLength,
				OpenRouterInputPrice:       zeroIfBlank(upstream.Pricing.Prompt),
				OpenRouterOutputPrice:      zeroIfBlank(upstream.Pricing.Completion),
				SDKMAXInputPrice:           sdkmaxInputPrice,
				SDKMAXOutputPrice:          sdkmaxOutputPrice,
				GlobalMultiplier:           globalMultiplierRaw,
				ModelMultiplier:            modelMultiplierRaw,
				UnifiedOpenRouterChannelID: channel.Id,
				Healthy:                    true,
				HealthMessage:              "available from OpenRouter models API",
				LastSyncedAt:               now,
				LastHealthCheckedAt:        now,
				UpdatedTime:                now,
			}
			if errors.Is(dbErr, gorm.ErrRecordNotFound) {
				row.CreatedTime = now
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
				result.ModelsCreated++
			} else if dbErr != nil {
				return dbErr
			} else {
				if err := tx.Model(&existing).Updates(row).Error; err != nil {
					return err
				}
				result.ModelsUpdated++
			}
			if err := model.UpsertModelAvailabilityFromUpstream(tx, deriveOpenRouterAvailability(upstream, now)); err != nil {
				return err
			}
		}

		if len(seen) > 0 {
			disabled := tx.Model(&model.OpenRouterModel{}).
				Where("model_id NOT IN ?", modelIDs).
				Updates(map[string]interface{}{
					"healthy":                false,
					"health_message":         "not returned by latest OpenRouter models API",
					"last_health_checked_at": now,
					"updated_time":           now,
				})
			if disabled.Error != nil {
				return disabled.Error
			}
			result.ModelsDisabled = int(disabled.RowsAffected)
			missingModelIDs := make([]string, 0)
			for _, existingID := range existingOpenRouterModelIDs {
				if _, ok := seen[existingID]; !ok {
					missingModelIDs = append(missingModelIDs, existingID)
				}
			}
			if err := model.MarkAvailabilityUpstreamMissing(tx, missingModelIDs, now); err != nil {
				return err
			}
		}

		sort.Strings(modelIDs)
		cleanupModelIDs = uniqueOpenRouterModelIDs(existingOpenRouterModelIDs, modelIDs)
		channel.Models = strings.Join(modelIDs, ",")
		if channel.Group == "" {
			channel.Group = "default"
		}
		if err := tx.Model(channel).Select("models", "group").Updates(map[string]interface{}{
			"models": channel.Models,
			"group":  channel.Group,
		}).Error; err != nil {
			return err
		}
		return channel.UpdateAbilities(tx)
	})
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}

	optionUpdates, err := RemoveOpenRouterManualPricingOptions(cleanupModelIDs)
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	modeBytes, err := common.Marshal(billingModes)
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	exprBytes, err := common.Marshal(billingExprs)
	if err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}
	optionUpdates["billing_setting.billing_mode"] = string(modeBytes)
	optionUpdates["billing_setting.billing_expr"] = string(exprBytes)
	if err := model.UpdateOptionsBulk(optionUpdates); err != nil {
		finishLog(model.OpenRouterSyncStatusFailed, err)
		return result, err
	}

	model.InvalidatePricingCache()
	finishLog(model.OpenRouterSyncStatusSuccess, nil)
	return result, nil
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

func parseMultiplier(value string, fallback float64) float64 {
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

func zeroIfBlank(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	return value
}

func StartOpenRouterModelSyncTask() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		for {
			common.OptionMapRWMutex.RLock()
			enabled := common.OptionMap["OpenRouterAutoSyncEnabled"] == "true"
			intervalRaw := common.OptionMap["OpenRouterAutoSyncIntervalMinutes"]
			common.OptionMapRWMutex.RUnlock()
			interval := common.String2Int(intervalRaw)
			if interval <= 0 {
				interval = 360
			}
			time.Sleep(time.Duration(interval) * time.Minute)
			common.OptionMapRWMutex.RLock()
			enabled = common.OptionMap["OpenRouterAutoSyncEnabled"] == "true"
			common.OptionMapRWMutex.RUnlock()
			if !enabled {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if _, err := SyncOpenRouterModels(ctx); err != nil {
				common.SysLog("openrouter auto sync failed: " + err.Error())
			}
			cancel()
		}
	}()
}
