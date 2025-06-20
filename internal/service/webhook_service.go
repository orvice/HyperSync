package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"butterfly.orx.me/core/log"
)

// WebhookService handles webhook endpoints for external sync triggers
type WebhookService struct {
	schedulerService *SchedulerService
	config           *WebhookConfig
}

// WebhookConfig contains webhook configuration
type WebhookConfig struct {
	Enabled        bool          // Whether webhooks are enabled
	Secret         string        // Webhook secret for signature verification
	AllowedSources []string      // Allowed webhook sources (e.g., "memos", "github")
	TrustedIPs     []string      // Trusted IP addresses (optional)
	Timeout        time.Duration // Webhook processing timeout
}

// WebhookPayload represents a webhook payload
type WebhookPayload struct {
	Source    string                 `json:"source"`    // Source system (e.g., "memos")
	Event     string                 `json:"event"`     // Event type (e.g., "memo.created", "memo.updated")
	Timestamp time.Time              `json:"timestamp"` // Event timestamp
	Data      map[string]interface{} `json:"data"`      // Event data
}

// WebhookResult represents the result of webhook processing
type WebhookResult struct {
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	TaskID      string    `json:"task_id,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}

// NewWebhookService creates a new webhook service
func NewWebhookService(schedulerService *SchedulerService, config *WebhookConfig) *WebhookService {
	if config == nil {
		config = &WebhookConfig{
			Enabled: false,
			Timeout: 30 * time.Second,
		}
	}

	return &WebhookService{
		schedulerService: schedulerService,
		config:           config,
	}
}

// HandleMemosWebhook handles webhooks from Memos
func (w *WebhookService) HandleMemosWebhook(ctx context.Context, r *http.Request) (*WebhookResult, error) {
	if !w.config.Enabled {
		return nil, fmt.Errorf("webhooks are disabled")
	}

	logger := log.FromContext(ctx)
	logger.Info("Received Memos webhook", "remote_addr", r.RemoteAddr)

	// Verify request
	if err := w.verifyRequest(r); err != nil {
		return nil, fmt.Errorf("webhook verification failed: %w", err)
	}

	// Parse payload
	payload, err := w.parsePayload(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Process webhook based on event type
	taskID, err := w.ProcessMemosEvent(ctx, payload)
	if err != nil {
		return &WebhookResult{
			Success:     false,
			Message:     err.Error(),
			ProcessedAt: time.Now(),
		}, nil
	}

	result := &WebhookResult{
		Success:     true,
		Message:     "Webhook processed successfully",
		TaskID:      taskID,
		ProcessedAt: time.Now(),
	}

	logger.Info("Memos webhook processed", "task_id", taskID, "event", payload.Event)
	return result, nil
}

// HandleGenericWebhook handles generic webhooks from various sources
func (w *WebhookService) HandleGenericWebhook(ctx context.Context, r *http.Request) (*WebhookResult, error) {
	if !w.config.Enabled {
		return nil, fmt.Errorf("webhooks are disabled")
	}

	logger := log.FromContext(ctx)
	logger.Info("Received generic webhook", "remote_addr", r.RemoteAddr, "user_agent", r.UserAgent())

	// Parse payload
	payload, err := w.parsePayload(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Verify source is allowed
	if !w.isSourceAllowed(payload.Source) {
		return nil, fmt.Errorf("source '%s' is not allowed", payload.Source)
	}

	// Process webhook based on source and event
	taskID, err := w.ProcessGenericEvent(ctx, payload)
	if err != nil {
		return &WebhookResult{
			Success:     false,
			Message:     err.Error(),
			ProcessedAt: time.Now(),
		}, nil
	}

	result := &WebhookResult{
		Success:     true,
		Message:     "Webhook processed successfully",
		TaskID:      taskID,
		ProcessedAt: time.Now(),
	}

	logger.Info("Generic webhook processed", "source", payload.Source, "event", payload.Event, "task_id", taskID)
	return result, nil
}

// verifyRequest verifies the webhook request signature and source
func (w *WebhookService) verifyRequest(r *http.Request) error {
	// Verify IP if trusted IPs are configured
	if len(w.config.TrustedIPs) > 0 {
		clientIP := w.getClientIP(r)
		if !w.isIPTrusted(clientIP) {
			return fmt.Errorf("IP %s is not trusted", clientIP)
		}
	}

	// Verify signature if secret is configured
	if w.config.Secret != "" {
		if err := w.verifySignature(r); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	return nil
}

// verifySignature verifies the webhook signature
func (w *WebhookService) verifySignature(r *http.Request) error {
	// Get signature from header
	signature := r.Header.Get("X-Webhook-Signature")
	if signature == "" {
		signature = r.Header.Get("X-Hub-Signature-256") // GitHub style
	}
	if signature == "" {
		return fmt.Errorf("no signature found in headers")
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}

	// Restore body for later reading
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(w.config.Secret))
	mac.Write(body)
	expectedSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// parsePayload parses the webhook payload
func (w *WebhookService) parsePayload(r *http.Request) (*WebhookPayload, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Set timestamp if not provided
	if payload.Timestamp.IsZero() {
		payload.Timestamp = time.Now()
	}

	return &payload, nil
}

// ProcessMemosEvent processes Memos-specific webhook events
func (w *WebhookService) ProcessMemosEvent(ctx context.Context, payload *WebhookPayload) (string, error) {
	logger := log.FromContext(ctx)

	// Determine sync trigger based on event type
	switch payload.Event {
	case "memo.created", "memo.updated":
		// Trigger sync for new or updated memos
		filters := &SyncFilters{
			SkipPrivate: true,
			MaxMemos:    10, // Limit for webhook-triggered syncs
		}

		// Check if specific memo ID is provided
		if memoID, ok := payload.Data["memo_id"].(string); ok && memoID != "" {
			logger.Info("Processing memo-specific webhook", "memo_id", memoID, "event", payload.Event)
			// Note: In a real implementation, you might want to sync only the specific memo
		}

		taskID, err := w.schedulerService.ScheduleTask(ctx, TaskTypeManualSync, PriorityHigh, []string{"mastodon", "bluesky"}, filters)
		if err != nil {
			return "", fmt.Errorf("failed to schedule sync task: %w", err)
		}

		return taskID, nil

	case "memo.deleted":
		// Note: For deleted memos, you might want to implement deletion on target platforms
		// This depends on your sync strategy and platform capabilities
		logger.Info("Memo deletion webhook received", "event", payload.Event)
		return "", fmt.Errorf("memo deletion not yet implemented")

	default:
		return "", fmt.Errorf("unsupported event type: %s", payload.Event)
	}
}

// ProcessGenericEvent processes generic webhook events
func (w *WebhookService) ProcessGenericEvent(ctx context.Context, payload *WebhookPayload) (string, error) {
	logger := log.FromContext(ctx)

	switch payload.Source {
	case "memos":
		return w.ProcessMemosEvent(ctx, payload)

	case "github":
		// Handle GitHub webhooks (e.g., for configuration updates)
		return w.processGitHubEvent(ctx, payload)

	case "manual":
		// Handle manual webhook triggers
		return w.processManualEvent(ctx, payload)

	default:
		logger.Warn("Unknown webhook source", "source", payload.Source)
		return "", fmt.Errorf("unsupported webhook source: %s", payload.Source)
	}
}

// processGitHubEvent processes GitHub webhook events
func (w *WebhookService) processGitHubEvent(ctx context.Context, payload *WebhookPayload) (string, error) {
	switch payload.Event {
	case "push":
		// Trigger sync on configuration changes
		filters := &SyncFilters{
			SkipPrivate: true,
			MaxMemos:    20,
		}

		taskID, err := w.schedulerService.ScheduleTask(ctx, TaskTypeManualSync, PriorityNormal, []string{"mastodon", "bluesky"}, filters)
		if err != nil {
			return "", fmt.Errorf("failed to schedule GitHub-triggered sync: %w", err)
		}

		return taskID, nil

	default:
		return "", fmt.Errorf("unsupported GitHub event: %s", payload.Event)
	}
}

// processManualEvent processes manual webhook triggers
func (w *WebhookService) processManualEvent(ctx context.Context, payload *WebhookPayload) (string, error) {
	// Extract parameters from payload data
	platforms := []string{"mastodon", "bluesky"} // Default platforms
	if platformsData, ok := payload.Data["platforms"].([]interface{}); ok {
		platforms = make([]string, len(platformsData))
		for i, p := range platformsData {
			if platform, ok := p.(string); ok {
				platforms[i] = platform
			}
		}
	}

	// Extract filters
	filters := &SyncFilters{
		SkipPrivate: true,
		MaxMemos:    50,
	}

	if filtersData, ok := payload.Data["filters"].(map[string]interface{}); ok {
		if skipPrivate, ok := filtersData["skip_private"].(bool); ok {
			filters.SkipPrivate = skipPrivate
		}
		if maxMemos, ok := filtersData["max_memos"].(float64); ok {
			filters.MaxMemos = int(maxMemos)
		}
		if creator, ok := filtersData["creator"].(string); ok {
			filters.MemosCreator = creator
		}
	}

	// Determine priority
	priority := PriorityNormal
	if priorityStr, ok := payload.Data["priority"].(string); ok {
		switch strings.ToLower(priorityStr) {
		case "low":
			priority = PriorityLow
		case "high":
			priority = PriorityHigh
		case "urgent":
			priority = PriorityUrgent
		}
	}

	taskID, err := w.schedulerService.ScheduleTask(ctx, TaskTypeManualSync, priority, platforms, filters)
	if err != nil {
		return "", fmt.Errorf("failed to schedule manual sync task: %w", err)
	}

	return taskID, nil
}

// isSourceAllowed checks if a webhook source is allowed
func (w *WebhookService) isSourceAllowed(source string) bool {
	if len(w.config.AllowedSources) == 0 {
		return true // Allow all sources if none specified
	}

	for _, allowed := range w.config.AllowedSources {
		if source == allowed {
			return true
		}
	}

	return false
}

// isIPTrusted checks if an IP address is trusted
func (w *WebhookService) isIPTrusted(ip string) bool {
	for _, trustedIP := range w.config.TrustedIPs {
		if ip == trustedIP {
			return true
		}
	}
	return false
}

// getClientIP extracts the client IP from the request
func (w *WebhookService) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}

// GetWebhookConfig returns the current webhook configuration
func (w *WebhookService) GetWebhookConfig() *WebhookConfig {
	return w.config
}

// UpdateWebhookConfig updates the webhook configuration
func (w *WebhookService) UpdateWebhookConfig(config *WebhookConfig) {
	w.config = config
}
