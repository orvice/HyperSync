package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"butterfly.orx.me/core/log"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SyncTriggerRequest defines the request body for triggering sync
type SyncTriggerRequest struct {
	Force           bool     `json:"force"`                      // Force sync even if recently synced
	TargetPlatforms []string `json:"target_platforms,omitempty"` // Override default target platforms
	MaxMemos        int      `json:"max_memos,omitempty"`        // Override max memos per run
}

// SyncTriggerResponse defines the response for sync trigger
type SyncTriggerResponse struct {
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	TotalMemosChecked  int      `json:"total_memos_checked"`
	MemosSynced        int      `json:"memos_synced"`
	MemosSkipped       int      `json:"memos_skipped"`
	SyncRecordsCreated int      `json:"sync_records_created"`
	SyncRecordsUpdated int      `json:"sync_records_updated"`
	Errors             []string `json:"errors,omitempty"`
}

// SyncStatusResponse defines the response for sync status query
type SyncStatusResponse struct {
	CurrentStatus string                   `json:"current_status"`
	PendingCount  int                      `json:"pending_count"`
	SyncedCount   int                      `json:"synced_count"`
	FailedCount   int                      `json:"failed_count"`
	LastSyncTime  *time.Time               `json:"last_sync_time,omitempty"`
	Config        map[string]interface{}   `json:"config"`
	RecentSynced  []map[string]interface{} `json:"recent_synced"`
	RecentFailed  []map[string]interface{} `json:"recent_failed"`
}

// SyncHistoryResponse defines the response for sync history query
type SyncHistoryResponse struct {
	Records    []map[string]interface{} `json:"records"`
	TotalCount int                      `json:"total_count"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	HasMore    bool                     `json:"has_more"`
}

// ConfigResponse defines the response for configuration query
type ConfigResponse struct {
	SyncConfig      map[string]interface{} `json:"sync_config"`
	SocialPlatforms map[string]interface{} `json:"social_platforms"`
}

// triggerSync handles POST /api/sync/trigger
func triggerSync(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	logger.Info("Manual sync triggered")

	syncService := GetSyncService()
	if syncService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sync service not initialized",
		})
		return
	}

	// Parse request body
	var req SyncTriggerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body: " + err.Error(),
		})
		return
	}

	// Validate target platforms if provided
	if len(req.TargetPlatforms) > 0 {
		validPlatforms := map[string]bool{"mastodon": true, "bluesky": true}
		for _, platform := range req.TargetPlatforms {
			if !validPlatforms[platform] {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Invalid target platform: " + platform,
				})
				return
			}
		}
	}

	// Start sync operation
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	result, err := syncService.SyncFromMemos(ctx)
	if err != nil {
		logger.Error("Sync operation failed", "error", err)
		c.JSON(http.StatusInternalServerError, SyncTriggerResponse{
			Success: false,
			Message: "Sync operation failed: " + err.Error(),
			Errors:  []string{err.Error()},
		})
		return
	}

	// Return success response
	response := SyncTriggerResponse{
		Success:            true,
		Message:            "Sync completed successfully",
		TotalMemosChecked:  result.TotalMemosChecked,
		MemosSynced:        result.MemosSynced,
		MemosSkipped:       result.MemosSkipped,
		SyncRecordsCreated: result.SyncRecordsCreated,
		SyncRecordsUpdated: result.SyncRecordsUpdated,
		Errors:             result.Errors,
	}

	logger.Info("Sync completed",
		"total_checked", result.TotalMemosChecked,
		"synced", result.MemosSynced,
		"skipped", result.MemosSkipped,
	)

	c.JSON(http.StatusOK, response)
}

// getSyncStatus handles GET /api/sync/status
func getSyncStatus(c *gin.Context) {
	syncService := GetSyncService()
	if syncService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sync service not initialized",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get sync status from service
	status, err := syncService.GetSyncStatus(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get sync status: " + err.Error(),
		})
		return
	}

	// Build response
	response := SyncStatusResponse{
		CurrentStatus: "ready", // TODO: Implement actual status tracking
		PendingCount:  status["pending_count"].(int),
		SyncedCount:   status["synced_count"].(int),
		FailedCount:   status["failed_count"].(int),
		Config:        status["config"].(map[string]interface{}),
		RecentSynced:  status["recent_synced"].([]map[string]interface{}),
		RecentFailed:  status["recent_failed"].([]map[string]interface{}),
	}

	c.JSON(http.StatusOK, response)
}

// getSyncHistory handles GET /api/sync/history
func getSyncHistory(c *gin.Context) {
	// Parse query parameters
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")
	status := c.Query("status")     // Optional filter by status
	platform := c.Query("platform") // Optional filter by platform

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	dao := GetMongoDAO()
	if dao == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database not initialized",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Build filter
	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}
	if platform != "" {
		filter["sync_targets."+platform] = bson.M{"$exists": true}
	}

	// Calculate skip
	skip := int64((page - 1) * pageSize)

	// Get records
	records, err := dao.ListSyncRecords(ctx, filter, int64(pageSize), skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to query sync history: " + err.Error(),
		})
		return
	}

	// Convert to response format
	var recordsData []map[string]interface{}
	for _, record := range records {
		recordData := map[string]interface{}{
			"id":              record.ID.Hex(),
			"source_platform": record.SourcePlatform,
			"source_id":       record.SourceID,
			"status":          record.Status,
			"content_preview": record.ContentPreview,
			"last_sync_at":    record.LastSyncAt,
			"created_at":      record.CreatedAt,
			"updated_at":      record.UpdatedAt,
			"error_message":   record.ErrorMessage,
			"retry_count":     record.RetryCount,
			"sync_targets":    record.SyncTargets,
		}
		recordsData = append(recordsData, recordData)
	}

	// Build response
	response := SyncHistoryResponse{
		Records:    recordsData,
		TotalCount: len(recordsData), // TODO: Get actual total count
		Page:       page,
		PageSize:   pageSize,
		HasMore:    len(recordsData) == pageSize, // Simple check
	}

	c.JSON(http.StatusOK, response)
}

// getConfig handles GET /api/config
func getConfig(c *gin.Context) {
	syncService := GetSyncService()
	socialService := GetSocialService()

	response := ConfigResponse{
		SyncConfig: map[string]interface{}{
			"status": "not_configured",
		},
		SocialPlatforms: map[string]interface{}{
			"status": "not_configured",
		},
	}

	// Get sync service config if available
	if syncService != nil {
		// TODO: Expose configuration from sync service
		response.SyncConfig = map[string]interface{}{
			"status":           "configured",
			"max_retries":      3,
			"batch_size":       20,
			"target_platforms": []string{"mastodon", "bluesky"},
		}
	}

	// Get social service config if available
	if socialService != nil {
		// TODO: Expose platform configuration from social service
		response.SocialPlatforms = map[string]interface{}{
			"status":    "configured",
			"platforms": []string{"mastodon", "bluesky"},
		}
	}

	c.JSON(http.StatusOK, response)
}

// updateConfig handles PUT /api/config
func updateConfig(c *gin.Context) {
	// TODO: Implement configuration update
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Configuration update not yet implemented",
	})
}

// retryFailedSyncs handles POST /api/sync/retry
func retryFailedSyncs(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	logger.Info("Retry failed syncs triggered")

	syncService := GetSyncService()
	if syncService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sync service not initialized",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	result, err := syncService.RetryFailedSyncs(ctx)
	if err != nil {
		logger.Error("Retry operation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Retry operation failed: " + err.Error(),
		})
		return
	}

	response := SyncTriggerResponse{
		Success:            true,
		Message:            "Retry completed",
		TotalMemosChecked:  result.TotalMemosChecked,
		MemosSynced:        result.MemosSynced,
		MemosSkipped:       result.MemosSkipped,
		SyncRecordsCreated: result.SyncRecordsCreated,
		SyncRecordsUpdated: result.SyncRecordsUpdated,
		Errors:             result.Errors,
	}

	c.JSON(http.StatusOK, response)
}
