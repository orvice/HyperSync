package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Posts processed counter
	SyncPostsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyper_sync_posts_processed_total",
			Help: "Total number of posts processed by sync service",
		},
		[]string{"main_social", "status"}, // status: processed, skipped_old, exists
	)

	// Cross-platform sync counter
	SyncCrossPostsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyper_sync_cross_posts_total",
			Help: "Total number of cross-platform post attempts",
		},
		[]string{"main_social", "target_platform", "status"}, // status: success, error
	)

	// Sync operation duration
	SyncOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hyper_sync_operation_duration_seconds",
			Help:    "Duration of sync operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"main_social", "operation"}, // operation: fetch_posts, sync_to_platform, total
	)

	// Database operations counter
	SyncDatabaseOpsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyper_sync_database_ops_total",
			Help: "Total number of database operations in sync service",
		},
		[]string{"main_social", "operation", "status"}, // operation: get_post, create_post, update_status
	)

	// Currently active sync operations
	SyncActiveOperations = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyper_sync_active_operations",
			Help: "Number of currently active sync operations",
		},
		[]string{"main_social"},
	)

	// Posts in queue gauge
	SyncPostsInQueue = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hyper_sync_posts_in_queue",
			Help: "Number of posts currently in sync queue",
		},
		[]string{"main_social"},
	)

	// Error rate by platform
	SyncErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyper_sync_errors_total",
			Help: "Total number of sync errors by type",
		},
		[]string{"main_social", "target_platform", "error_type"}, // error_type: platform_error, database_error, network_error
	)

	// Retry attempts counter
	SyncRetriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hyper_sync_retries_total",
			Help: "Total number of retry attempts",
		},
		[]string{"main_social", "target_platform"},
	)
)

// Labels for different status values
const (
	StatusProcessed  = "processed"
	StatusSkippedOld = "skipped_old"
	StatusExists     = "exists"
	StatusSuccess    = "success"
	StatusError      = "error"

	OperationFetchPosts     = "fetch_posts"
	OperationSyncToPlatform = "sync_to_platform"
	OperationTotal          = "total"
	OperationGetPost        = "get_post"
	OperationCreatePost     = "create_post"
	OperationUpdateStatus   = "update_status"

	ErrorTypePlatform = "platform_error"
	ErrorTypeDatabase = "database_error"
	ErrorTypeNetwork  = "network_error"
	ErrorTypeGeneral  = "general_error"
)
