package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"butterfly.orx.me/core/log"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// SyncService handles synchronization between Memos and social platforms
type SyncService struct {
	dao           *dao.MongoDAO
	converter     *ContentConverter
	socialService *SocialService
	memosClient   *social.Memos
	syncConfig    *SyncConfig
}

// SyncConfig contains configuration for the sync service
type SyncConfig struct {
	MaxRetries      int           // Maximum retry attempts for failed syncs
	SyncInterval    time.Duration // Interval between sync runs
	BatchSize       int           // Number of memos to process in each batch
	MaxMemosPerRun  int           // Maximum memos to sync in one run
	TargetPlatforms []string      // Platforms to sync to
	MemosConfig     *MemosConfig  // Memos connection config
	SkipPrivate     bool          // Skip private memos
	SkipOlder       time.Duration // Skip memos older than this duration
}

// MemosConfig contains Memos-specific configuration
type MemosConfig struct {
	Endpoint string
	Token    string
	Creator  string // Optional: filter by creator
}

// SyncResult contains the result of a sync operation
type SyncResult struct {
	TotalMemosChecked  int
	NewMemosFound      int
	MemosSkipped       int
	MemosSynced        int
	MemosSkippedError  int
	SyncRecordsCreated int
	SyncRecordsUpdated int
	Errors             []string
}

// NewSyncService creates a new sync service
func NewSyncService(dao *dao.MongoDAO, socialService *SocialService, config *SyncConfig) (*SyncService, error) {
	if config == nil {
		return nil, fmt.Errorf("sync config cannot be nil")
	}

	if config.MemosConfig == nil {
		return nil, fmt.Errorf("memos config cannot be nil")
	}

	// Create Memos client
	memosClient := social.NewMemos(config.MemosConfig.Endpoint, config.MemosConfig.Token)

	// Create content converter
	converter := NewContentConverter()

	return &SyncService{
		dao:           dao,
		converter:     converter,
		socialService: socialService,
		memosClient:   memosClient,
		syncConfig:    config,
	}, nil
}

// SyncFromMemos performs a complete sync from Memos to configured platforms
func (s *SyncService) SyncFromMemos(ctx context.Context) (*SyncResult, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting Memos sync", "target_platforms", s.syncConfig.TargetPlatforms)

	result := &SyncResult{}

	// Fetch memos from Memos platform
	memos, err := s.fetchNewMemos(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to fetch memos: %v", err))
		return result, fmt.Errorf("failed to fetch memos: %w", err)
	}

	result.TotalMemosChecked = len(memos)
	logger.Info("Fetched memos", "count", len(memos))

	// Process each memo
	for _, memo := range memos {
		processed, err := s.processMemo(ctx, memo, result)
		if err != nil {
			logger.Error("Failed to process memo", "memo_id", memo.Name, "error", err)
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to process memo %s: %v", memo.Name, err))
			continue
		}

		if processed {
			result.MemosSynced++
		} else {
			result.MemosSkipped++
		}

		// Stop if we've reached the maximum per run
		if s.syncConfig.MaxMemosPerRun > 0 && result.MemosSynced >= s.syncConfig.MaxMemosPerRun {
			logger.Info("Reached maximum memos per run", "limit", s.syncConfig.MaxMemosPerRun)
			break
		}
	}

	logger.Info("Memos sync completed",
		"total_checked", result.TotalMemosChecked,
		"synced", result.MemosSynced,
		"skipped", result.MemosSkipped,
		"errors", len(result.Errors),
	)

	return result, nil
}

// fetchNewMemos retrieves new memos from the Memos platform
func (s *SyncService) fetchNewMemos(ctx context.Context) ([]*social.Memo, error) {
	// Prepare request parameters
	req := &social.ListMemosRequest{
		PageSize: s.syncConfig.BatchSize,
		OrderBy:  "display_time desc", // Get newest first
	}

	// Filter by creator if specified
	if s.syncConfig.MemosConfig.Creator != "" {
		req.Creator = s.syncConfig.MemosConfig.Creator
	}

	// Filter visibility if needed
	if s.syncConfig.SkipPrivate {
		req.Visibility = "PUBLIC"
	}

	// Fetch memos
	response, err := s.memosClient.ListMemos(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list memos: %w", err)
	}

	var newMemos []*social.Memo

	// Filter memos based on age and existing sync records
	for _, memo := range response.Memos {
		// Skip if too old
		if s.syncConfig.SkipOlder > 0 {
			if time.Since(memo.DisplayTime) > s.syncConfig.SkipOlder {
				continue
			}
		}

		// Skip private memos if configured
		if s.syncConfig.SkipPrivate && strings.ToUpper(memo.Visibility) == "PRIVATE" {
			continue
		}

		// Check if we've already synced this memo
		sourceID := extractMemoID(memo.Name)
		existingRecord, err := s.dao.GetSyncRecordBySource(ctx, "memos", sourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing sync record: %w", err)
		}

		// Skip if already successfully synced and content hasn't changed
		if existingRecord != nil && existingRecord.Status == dao.SyncStatusSynced {
			currentHash := s.generateContentHash(memo.Content)
			if existingRecord.ContentHash == currentHash {
				continue // Already synced and unchanged
			}
		}

		newMemos = append(newMemos, &memo)
	}

	return newMemos, nil
}

// processMemo processes a single memo for synchronization
func (s *SyncService) processMemo(ctx context.Context, memo *social.Memo, result *SyncResult) (bool, error) {
	logger := log.FromContext(ctx)
	sourceID := extractMemoID(memo.Name)

	logger.Debug("Processing memo", "memo_id", sourceID, "content_preview", s.getContentPreview(memo.Content))

	// Convert memo to post
	post, err := s.converter.MemoToPost(memo)
	if err != nil {
		return false, fmt.Errorf("failed to convert memo to post: %w", err)
	}

	// Generate content hash
	contentHash := s.generateContentHash(memo.Content)

	// Check for existing sync record
	existingRecord, err := s.dao.GetSyncRecordBySource(ctx, "memos", sourceID)
	if err != nil {
		return false, fmt.Errorf("failed to check existing sync record: %w", err)
	}

	var syncRecordID string

	if existingRecord != nil {
		// Update existing record
		existingRecord.ContentHash = contentHash
		existingRecord.ContentPreview = s.getContentPreview(memo.Content)
		existingRecord.Status = dao.SyncStatusPending
		existingRecord.RetryCount = 0
		existingRecord.ErrorMessage = ""

		if err := s.dao.UpdateSyncRecord(ctx, existingRecord); err != nil {
			return false, fmt.Errorf("failed to update sync record: %w", err)
		}
		syncRecordID = existingRecord.ID.Hex()
		result.SyncRecordsUpdated++
	} else {
		// Create new sync record
		syncRecord := &dao.SyncRecordModel{
			SourcePlatform: "memos",
			SourceID:       sourceID,
			Status:         dao.SyncStatusPending,
			MaxRetries:     s.syncConfig.MaxRetries,
			ContentHash:    contentHash,
			ContentPreview: s.getContentPreview(memo.Content),
		}

		id, err := s.dao.CreateSyncRecord(ctx, syncRecord)
		if err != nil {
			return false, fmt.Errorf("failed to create sync record: %w", err)
		}
		syncRecordID = id
		result.SyncRecordsCreated++
	}

	// Sync to each target platform
	syncSuccess := false
	for _, platform := range s.syncConfig.TargetPlatforms {
		err := s.syncToTargetPlatform(ctx, post, platform, syncRecordID)
		if err != nil {
			logger.Error("Failed to sync to platform", "platform", platform, "memo_id", sourceID, "error", err)

			// Update target status
			targetStatus := dao.SyncTargetStatus{
				Status:     dao.SyncStatusFailed,
				Error:      err.Error(),
				RetryCount: 1,
			}
			if updateErr := s.dao.UpdateSyncTargetStatus(ctx, syncRecordID, platform, targetStatus); updateErr != nil {
				logger.Error("Failed to update target status", "error", updateErr)
			}

			result.Errors = append(result.Errors, fmt.Sprintf("Platform %s: %v", platform, err))
		} else {
			syncSuccess = true
			logger.Info("Successfully synced to platform", "platform", platform, "memo_id", sourceID)
		}
	}

	// Update overall sync status
	if syncSuccess {
		if err := s.dao.UpdateSyncRecordStatus(ctx, syncRecordID, dao.SyncStatusSynced, ""); err != nil {
			logger.Error("Failed to update sync record status", "error", err)
		}
		return true, nil
	} else {
		if err := s.dao.UpdateSyncRecordStatus(ctx, syncRecordID, dao.SyncStatusFailed, "Failed to sync to any target platform"); err != nil {
			logger.Error("Failed to update sync record status", "error", err)
		}
		return false, fmt.Errorf("failed to sync to any target platform")
	}
}

// syncToTargetPlatform synchronizes a post to a specific target platform
func (s *SyncService) syncToTargetPlatform(ctx context.Context, post *social.Post, platform, syncRecordID string) error {
	// Post to the platform via social service
	resp, err := s.socialService.PostToPlatform(ctx, platform, post)
	if err != nil {
		return fmt.Errorf("failed to post to %s: %w", platform, err)
	}

	// Extract platform ID from response
	var platformID string
	switch v := resp.(type) {
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			platformID = id
		} else if uri, ok := v["uri"].(string); ok {
			// For platforms like Bluesky that return URI
			platformID = uri
		}
	case string:
		platformID = v
	}

	// Update target status
	now := time.Now()
	targetStatus := dao.SyncTargetStatus{
		Status:     dao.SyncStatusSynced,
		PlatformID: platformID,
		SyncedAt:   &now,
	}

	return s.dao.UpdateSyncTargetStatus(ctx, syncRecordID, platform, targetStatus)
}

// RetryFailedSyncs retries synchronization for failed records
func (s *SyncService) RetryFailedSyncs(ctx context.Context) (*SyncResult, error) {
	logger := log.FromContext(ctx)
	logger.Info("Starting retry of failed syncs")

	result := &SyncResult{}

	// Get pending and failed records that haven't exceeded max retries
	records, err := s.dao.GetPendingSyncRecords(ctx, s.syncConfig.MaxRetries)
	if err != nil {
		return result, fmt.Errorf("failed to get pending sync records: %w", err)
	}

	result.TotalMemosChecked = len(records)

	for _, record := range records {
		// Fetch the original memo to re-sync
		// This is a simplified approach - in practice, you might want to cache this
		logger.Info("Retrying sync", "source_id", record.SourceID, "retry_count", record.RetryCount)

		// For now, just mark as skipped since we'd need the original memo data
		// In a full implementation, you'd either:
		// 1. Store the memo data in the sync record
		// 2. Re-fetch from Memos (but risk it being deleted/changed)
		// 3. Have a separate memo cache

		if err := s.dao.UpdateSyncRecordStatus(ctx, record.ID.Hex(), dao.SyncStatusSkipped, "Retry not implemented - original memo data not available"); err != nil {
			logger.Error("Failed to update sync record", "error", err)
		}
		result.MemosSkipped++
	}

	return result, nil
}

// generateContentHash creates a hash of the memo content for change detection
func (s *SyncService) generateContentHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// getContentPreview returns the first 100 characters of content for preview
func (s *SyncService) getContentPreview(content string) string {
	content = strings.TrimSpace(content)
	if len(content) > 100 {
		return content[:100] + "..."
	}
	return content
}

// GetSyncStatus returns the current sync status and statistics
func (s *SyncService) GetSyncStatus(ctx context.Context) (map[string]interface{}, error) {
	// Get sync record counts by status
	pending, err := s.dao.GetSyncRecordsByStatus(ctx, dao.SyncStatusPending, 0)
	if err != nil {
		return nil, err
	}

	synced, err := s.dao.GetSyncRecordsByStatus(ctx, dao.SyncStatusSynced, 10) // Limit to recent 10
	if err != nil {
		return nil, err
	}

	failed, err := s.dao.GetSyncRecordsByStatus(ctx, dao.SyncStatusFailed, 10)
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"pending_count": len(pending),
		"synced_count":  len(synced),
		"failed_count":  len(failed),
		"config": map[string]interface{}{
			"target_platforms": s.syncConfig.TargetPlatforms,
			"max_retries":      s.syncConfig.MaxRetries,
			"batch_size":       s.syncConfig.BatchSize,
		},
		"recent_synced": extractSyncRecordSummaries(synced),
		"recent_failed": extractSyncRecordSummaries(failed),
	}

	return status, nil
}

// extractSyncRecordSummaries extracts summary information from sync records
func extractSyncRecordSummaries(records []*dao.SyncRecordModel) []map[string]interface{} {
	summaries := make([]map[string]interface{}, len(records))
	for i, record := range records {
		summaries[i] = map[string]interface{}{
			"id":              record.ID.Hex(),
			"source_id":       record.SourceID,
			"status":          record.Status,
			"content_preview": record.ContentPreview,
			"last_sync_at":    record.LastSyncAt,
			"error_message":   record.ErrorMessage,
		}
	}
	return summaries
}
