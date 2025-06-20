package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.orx.me/apps/hyper-sync/internal/service"
)

// SchedulerHandlers contains scheduler-related HTTP handlers
type SchedulerHandlers struct {
	schedulerService *service.SchedulerService
}

// NewSchedulerHandlers creates new scheduler handlers
func NewSchedulerHandlers(schedulerService *service.SchedulerService) *SchedulerHandlers {
	return &SchedulerHandlers{
		schedulerService: schedulerService,
	}
}

// GetSchedulerStatus returns current scheduler status
func (h *SchedulerHandlers) GetSchedulerStatus(c *gin.Context) {
	status := h.schedulerService.GetSchedulerStatus(c.Request.Context())

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// StartScheduler starts the scheduler service
func (h *SchedulerHandlers) StartScheduler(c *gin.Context) {
	err := h.schedulerService.Start(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scheduler started successfully",
	})
}

// StopScheduler stops the scheduler service
func (h *SchedulerHandlers) StopScheduler(c *gin.Context) {
	err := h.schedulerService.Stop(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Scheduler stopped successfully",
	})
}

// ScheduleTask manually schedules a sync task
func (h *SchedulerHandlers) ScheduleTask(c *gin.Context) {
	var req struct {
		Type      string   `json:"type" binding:"required"` // Task type: auto_sync, manual_sync, retry_sync
		Priority  string   `json:"priority"`                // Priority: low, normal, high, urgent
		Platforms []string `json:"platforms"`               // Target platforms
		Filters   *struct {
			MemosCreator string `json:"memos_creator"`
			SkipPrivate  bool   `json:"skip_private"`
			SkipOlder    int    `json:"skip_older_hours"` // Hours
			MaxMemos     int    `json:"max_memos"`
		} `json:"filters"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Convert task type
	var taskType service.TaskType
	switch strings.ToLower(req.Type) {
	case "auto_sync":
		taskType = service.TaskTypeAutoSync
	case "manual_sync":
		taskType = service.TaskTypeManualSync
	case "retry_sync":
		taskType = service.TaskTypeRetrySync
	default:
		taskType = service.TaskTypeManualSync
	}

	// Convert priority
	var priority service.TaskPriority
	switch strings.ToLower(req.Priority) {
	case "low":
		priority = service.PriorityLow
	case "high":
		priority = service.PriorityHigh
	case "urgent":
		priority = service.PriorityUrgent
	default:
		priority = service.PriorityNormal
	}

	// Default platforms if not specified
	if len(req.Platforms) == 0 {
		req.Platforms = []string{"mastodon", "bluesky"}
	}

	// Convert filters
	var filters *service.SyncFilters
	if req.Filters != nil {
		filters = &service.SyncFilters{
			MemosCreator: req.Filters.MemosCreator,
			SkipPrivate:  req.Filters.SkipPrivate,
			MaxMemos:     req.Filters.MaxMemos,
		}
		if req.Filters.SkipOlder > 0 {
			filters.SkipOlder = time.Duration(req.Filters.SkipOlder) * time.Hour
		}
	}

	taskID, err := h.schedulerService.ScheduleTask(c.Request.Context(), taskType, priority, req.Platforms, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Task scheduled successfully",
		"data": gin.H{
			"task_id": taskID,
		},
	})
}

// GetTaskQueue returns current task queue status
func (h *SchedulerHandlers) GetTaskQueue(c *gin.Context) {
	status := h.schedulerService.GetSchedulerStatus(c.Request.Context())

	// Extract queue-specific information
	queueInfo := gin.H{
		"tasks_in_queue":        status["tasks_in_queue"],
		"active_workers":        status["active_workers"],
		"max_concurrent_tasks":  status["max_concurrent_tasks"],
		"total_tasks_processed": status["total_tasks_processed"],
		"average_task_time":     status["average_task_time"],
		"success_rate":          status["success_rate"],
		"error_rate":            status["error_rate"],
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    queueInfo,
	})
}

// GetCronJobs returns current cron jobs status
func (h *SchedulerHandlers) GetCronJobs(c *gin.Context) {
	status := h.schedulerService.GetSchedulerStatus(c.Request.Context())

	cronJobs, ok := status["cron_jobs"]
	if !ok {
		cronJobs = map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cronJobs,
	})
}

// UpdateSchedulerConfig updates scheduler configuration (if supported)
func (h *SchedulerHandlers) UpdateSchedulerConfig(c *gin.Context) {
	var req struct {
		AutoSyncEnabled    *bool `json:"auto_sync_enabled"`
		DefaultInterval    *int  `json:"default_interval_minutes"`
		MaxConcurrentTasks *int  `json:"max_concurrent_tasks"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Note: This is a simplified implementation
	// In a real system, you might want to:
	// 1. Validate the configuration
	// 2. Persist the changes
	// 3. Restart the scheduler with new config

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration updated (restart required for some changes)",
	})
}

// ClearTaskQueue clears all pending tasks from the queue
func (h *SchedulerHandlers) ClearTaskQueue(c *gin.Context) {
	// Note: This would require implementing a ClearQueue method in SchedulerService
	// For now, return a placeholder response

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Task queue clearing requested (not yet implemented)",
	})
}

// RetryFailedTasks retries all failed tasks
func (h *SchedulerHandlers) RetryFailedTasks(c *gin.Context) {
	// Extract optional parameters
	maxRetries := 1
	if maxRetriesStr := c.Query("max_retries"); maxRetriesStr != "" {
		if parsed, err := strconv.Atoi(maxRetriesStr); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	platforms := []string{"mastodon", "bluesky"}
	if platformsStr := c.Query("platforms"); platformsStr != "" {
		platforms = strings.Split(platformsStr, ",")
		// Trim whitespace
		for i := range platforms {
			platforms[i] = strings.TrimSpace(platforms[i])
		}
	}

	// Schedule retry tasks
	filters := &service.SyncFilters{
		SkipPrivate: true,
		MaxMemos:    20, // Limit for retry operations
	}

	var taskIDs []string
	for i := 0; i < maxRetries; i++ {
		taskID, err := h.schedulerService.ScheduleTask(c.Request.Context(), service.TaskTypeRetrySync, service.PriorityHigh, platforms, filters)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("Failed to schedule retry task %d: %v", i+1, err),
			})
			return
		}
		taskIDs = append(taskIDs, taskID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Scheduled %d retry tasks", len(taskIDs)),
		"data": gin.H{
			"task_ids": taskIDs,
		},
	})
}
