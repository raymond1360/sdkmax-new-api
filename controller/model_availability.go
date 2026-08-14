package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type updateModelAvailabilityAdminRequest struct {
	ModelID      string `json:"model_id"`
	AdminEnabled bool   `json:"admin_enabled"`
}

type triggerModelAvailabilityRetestRequest struct {
	ModelID string `json:"model_id"`
}

func ListModelAvailability(c *gin.Context) {
	page := common.String2Int(c.DefaultQuery("page", "1"))
	pageSize := common.String2Int(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	query := model.DB.Model(&model.ModelAvailability{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("model_id LIKE ?", "%"+keyword+"%")
	}
	if status := strings.TrimSpace(c.Query("connectivity_status")); status != "" {
		query = query.Where("connectivity_status = ?", status)
	}
	if support := strings.TrimSpace(c.Query("sdkmax_support_status")); support != "" {
		query = query.Where("sdkmax_support_status = ?", support)
	}
	if visible := strings.TrimSpace(c.Query("customer_visible")); visible != "" {
		query = query.Where("customer_visible = ?", visible == "true" || visible == "1")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	var rows []model.ModelAvailability
	if err := query.Order("customer_visible ASC").Order("updated_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"total":   total,
	})
}

func UpdateModelAvailabilityAdminEnabled(c *gin.Context) {
	var req updateModelAvailabilityAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	operatorID := c.GetInt("id")
	operatorName, _ := model.GetUsernameById(operatorID, false)
	row, err := model.SetModelAvailabilityAdminEnabled(req.ModelID, req.AdminEnabled, operatorID, operatorName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func TriggerModelAvailabilityRetest(c *gin.Context) {
	var req triggerModelAvailabilityRetestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "model_id is required"})
		return
	}
	now := common.GetTimestamp()
	operatorID := c.GetInt("id")
	operatorName, _ := model.GetUsernameById(operatorID, false)
	row, err := model.MarkModelAvailabilityRetestRequested(modelID, operatorID, operatorName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	model.InvalidatePricingCache()
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      row,
		"message":   "model retest requested; current visibility is preserved until a health checker records the final result",
		"queued_at": now,
	})
}

func BootstrapModelAvailabilityFromAudit(c *gin.Context) {
	count, err := model.BootstrapModelAvailabilityFromAuditCSV("")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "created": count})
}
