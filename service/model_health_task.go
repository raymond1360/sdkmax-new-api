package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const modelHealthDefaultIntervalMinutes = 360

var modelHealthTaskOnce sync.Once

func modelHealthOptionBool(primary, fallback string, defaultValue bool) bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if value, ok := common.OptionMap[primary]; ok {
		return value == "true"
	}
	if value, ok := common.OptionMap[fallback]; ok {
		return value == "true"
	}
	return defaultValue
}

func modelHealthOptionInt(key string, defaultValue int) int {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func StartModelHealthCheckTask() {
	if !common.IsMasterNode {
		return
	}
	modelHealthTaskOnce.Do(func() {
		go func() {
			for {
				interval := modelHealthOptionInt("ModelHealthCheckIntervalMinutes", modelHealthDefaultIntervalMinutes)
				time.Sleep(time.Duration(interval) * time.Minute)
				if !modelHealthOptionBool("MODEL_HEALTH_CHECK_ENABLED", "ModelHealthCheckEnabled", false) {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(modelHealthOptionInt("ModelHealthCheckTimeoutSeconds", 45))*time.Second)
				if err := RunModelHealthCheckSweep(ctx); err != nil {
					common.SysLog("model health check sweep failed: " + err.Error())
				}
				cancel()
			}
		}()
	})
}

func RunModelHealthCheckSweep(ctx context.Context) error {
	if model.DB == nil {
		return nil
	}
	if !model.ModelAvailabilityTableExists() {
		return nil
	}
	limit := modelHealthOptionInt("ModelHealthCheckConcurrency", 3)
	if limit <= 0 {
		limit = 3
	}
	var rows []model.ModelAvailability
	if err := model.DB.
		Where("admin_enabled = ? AND sdkmax_support_status = ? AND api_mode = ?", true, model.SupportSupported, model.APIModeRealtime).
		Where("connectivity_status IN ?", []string{model.ConnectivityUntested, model.ConnectivityDegraded, model.ConnectivityFailed, model.ConnectivityTesting}).
		Order("last_tested_at ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		common.SysLog(fmt.Sprintf("model health check sweep selected %d models; upstream tester dispatch is pending integration", len(rows)))
		return nil
	}
}
