package metrics

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.Meter("hypersync")

// MustInt64Counter panics on error; safe at init time.
func mustInt64Counter(name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}

func mustFloat64Histogram(name, desc, unit string) metric.Float64Histogram {
	h, err := meter.Float64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit))
	if err != nil {
		panic(err)
	}
	return h
}

func mustInt64UpDownCounter(name, desc string) metric.Int64UpDownCounter {
	c, err := meter.Int64UpDownCounter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}

func mustInt64Gauge(name, desc string) metric.Int64Gauge {
	g, err := meter.Int64Gauge(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return g
}

var (
	PostsProcessedTotal = mustInt64Counter(
		"hyper_sync_posts_processed_total",
		"Total number of posts processed by sync service",
	)
	CrossPostsTotal = mustInt64Counter(
		"hyper_sync_cross_posts_total",
		"Total number of cross-platform post attempts",
	)
	OperationDuration = mustFloat64Histogram(
		"hyper_sync_operation_duration_seconds",
		"Duration of sync operations",
		"s",
	)
	DatabaseOpsTotal = mustInt64Counter(
		"hyper_sync_database_ops_total",
		"Total number of database operations in sync service",
	)
	ActiveOperations = mustInt64UpDownCounter(
		"hyper_sync_active_operations",
		"Number of currently active sync operations",
	)
	PostsInQueue = mustInt64Gauge(
		"hyper_sync_posts_in_queue",
		"Number of posts currently in sync queue",
	)
	ErrorsTotal = mustInt64Counter(
		"hyper_sync_errors_total",
		"Total number of sync errors by type",
	)
	RetriesTotal = mustInt64Counter(
		"hyper_sync_retries_total",
		"Total number of retry attempts",
	)
)

const (
	StatusProcessed     = "processed"
	StatusSkippedOld    = "skipped_old"
	StatusSkippedDirect = "skipped_direct"
	StatusExists        = "exists"
	StatusSuccess       = "success"
	StatusError         = "error"

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
