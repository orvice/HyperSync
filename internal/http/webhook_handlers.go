package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.orx.me/apps/hyper-sync/internal/service"
)

// WebhookHandlers contains webhook-related HTTP handlers
type WebhookHandlers struct {
	webhookService *service.WebhookService
}

// NewWebhookHandlers creates new webhook handlers
func NewWebhookHandlers(webhookService *service.WebhookService) *WebhookHandlers {
	return &WebhookHandlers{
		webhookService: webhookService,
	}
}

// HandleMemosWebhook handles webhooks from Memos
func (h *WebhookHandlers) HandleMemosWebhook(c *gin.Context) {
	result, err := h.webhookService.HandleMemosWebhook(c.Request.Context(), c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if result.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": result.Message,
			"data": gin.H{
				"task_id":      result.TaskID,
				"processed_at": result.ProcessedAt,
			},
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   result.Message,
			"data": gin.H{
				"processed_at": result.ProcessedAt,
			},
		})
	}
}

// HandleGenericWebhook handles generic webhooks from various sources
func (h *WebhookHandlers) HandleGenericWebhook(c *gin.Context) {
	result, err := h.webhookService.HandleGenericWebhook(c.Request.Context(), c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if result.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": result.Message,
			"data": gin.H{
				"task_id":      result.TaskID,
				"processed_at": result.ProcessedAt,
			},
		})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   result.Message,
			"data": gin.H{
				"processed_at": result.ProcessedAt,
			},
		})
	}
}

// GetWebhookConfig returns the current webhook configuration
func (h *WebhookHandlers) GetWebhookConfig(c *gin.Context) {
	config := h.webhookService.GetWebhookConfig()

	// Hide sensitive information
	safeConfig := gin.H{
		"enabled":         config.Enabled,
		"allowed_sources": config.AllowedSources,
		"trusted_ips":     config.TrustedIPs,
		"timeout":         config.Timeout.String(),
		"has_secret":      config.Secret != "",
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    safeConfig,
	})
}

// UpdateWebhookConfig updates webhook configuration
func (h *WebhookHandlers) UpdateWebhookConfig(c *gin.Context) {
	var req struct {
		Enabled        *bool    `json:"enabled"`
		Secret         *string  `json:"secret"`
		AllowedSources []string `json:"allowed_sources"`
		TrustedIPs     []string `json:"trusted_ips"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Get current config
	currentConfig := h.webhookService.GetWebhookConfig()

	// Create new config with updates
	newConfig := &service.WebhookConfig{
		Enabled:        currentConfig.Enabled,
		Secret:         currentConfig.Secret,
		AllowedSources: currentConfig.AllowedSources,
		TrustedIPs:     currentConfig.TrustedIPs,
		Timeout:        currentConfig.Timeout,
	}

	// Apply updates
	if req.Enabled != nil {
		newConfig.Enabled = *req.Enabled
	}
	if req.Secret != nil {
		newConfig.Secret = *req.Secret
	}
	if req.AllowedSources != nil {
		newConfig.AllowedSources = req.AllowedSources
	}
	if req.TrustedIPs != nil {
		newConfig.TrustedIPs = req.TrustedIPs
	}

	// Update configuration
	h.webhookService.UpdateWebhookConfig(newConfig)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Webhook configuration updated successfully",
	})
}

// TestWebhook allows testing webhook functionality
func (h *WebhookHandlers) TestWebhook(c *gin.Context) {
	var req struct {
		Source string                 `json:"source" binding:"required"`
		Event  string                 `json:"event" binding:"required"`
		Data   map[string]interface{} `json:"data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Create a test webhook payload
	testPayload := &service.WebhookPayload{
		Source: req.Source,
		Event:  req.Event,
		Data:   req.Data,
	}

	// Process based on source
	var taskID string
	var err error

	switch req.Source {
	case "memos":
		taskID, err = h.webhookService.ProcessMemosEvent(c.Request.Context(), testPayload)
	default:
		taskID, err = h.webhookService.ProcessGenericEvent(c.Request.Context(), testPayload)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Test webhook processed successfully",
		"data": gin.H{
			"task_id": taskID,
		},
	})
}

// GetWebhookStats returns webhook statistics
func (h *WebhookHandlers) GetWebhookStats(c *gin.Context) {
	// Note: This would require implementing statistics tracking in WebhookService
	// For now, return placeholder data

	stats := gin.H{
		"total_webhooks_received": 0,
		"successful_webhooks":     0,
		"failed_webhooks":         0,
		"webhooks_by_source": gin.H{
			"memos":  0,
			"github": 0,
			"manual": 0,
		},
		"recent_webhooks": []gin.H{},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}
