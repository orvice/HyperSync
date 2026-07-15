package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Attribute keys used across all metrics.
const (
	AttrMainSocial     = "main_social"
	AttrTargetPlatform = "target_platform"
	AttrStatus         = "status"
	AttrOperation      = "operation"
	AttrErrorType      = "error_type"
)

// SyncMetrics provides a convenient wrapper for sync-related metrics.
type SyncMetrics struct {
	mainSocial attribute.KeyValue
}

func NewSyncMetrics(mainSocial string) *SyncMetrics {
	return &SyncMetrics{
		mainSocial: attribute.String(AttrMainSocial, mainSocial),
	}
}

// Timer represents a timing measurement.
type Timer struct {
	attrs []attribute.KeyValue
	start time.Time
}

func (m *SyncMetrics) NewTimer(operation string) *Timer {
	return &Timer{
		attrs: []attribute.KeyValue{
			m.mainSocial,
			attribute.String(AttrOperation, operation),
		},
		start: time.Now(),
	}
}

func (t *Timer) Stop() {
	OperationDuration.Record(context.Background(), time.Since(t.start).Seconds(),
		metric.WithAttributes(t.attrs...))
}

func (m *SyncMetrics) IncPostsProcessed(status string) {
	PostsProcessedTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial, attribute.String(AttrStatus, status)))
}

func (m *SyncMetrics) IncCrossPosts(targetPlatform, status string) {
	CrossPostsTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial,
			attribute.String(AttrTargetPlatform, targetPlatform),
			attribute.String(AttrStatus, status)))
}

func (m *SyncMetrics) IncDatabaseOps(operation, status string) {
	DatabaseOpsTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial,
			attribute.String(AttrOperation, operation),
			attribute.String(AttrStatus, status)))
}

func (m *SyncMetrics) IncErrors(targetPlatform, errorType string) {
	ErrorsTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial,
			attribute.String(AttrTargetPlatform, targetPlatform),
			attribute.String(AttrErrorType, errorType)))
}

func (m *SyncMetrics) IncRetries(targetPlatform string) {
	RetriesTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial,
			attribute.String(AttrTargetPlatform, targetPlatform)))
}

func (m *SyncMetrics) SetPostsInQueue(count int) {
	PostsInQueue.Record(context.Background(), int64(count),
		metric.WithAttributes(m.mainSocial))
}

func (m *SyncMetrics) IncActiveOperations() {
	ActiveOperations.Add(context.Background(), 1,
		metric.WithAttributes(m.mainSocial))
}

func (m *SyncMetrics) DecActiveOperations() {
	ActiveOperations.Add(context.Background(), -1,
		metric.WithAttributes(m.mainSocial))
}

func (m *SyncMetrics) ActiveOperationsContext(ctx context.Context, fn func(context.Context) error) error {
	m.IncActiveOperations()
	defer m.DecActiveOperations()
	return fn(ctx)
}

func (m *SyncMetrics) TimedOperation(operation string, fn func() error) error {
	timer := m.NewTimer(operation)
	defer timer.Stop()
	return fn()
}

func (m *SyncMetrics) TimedOperationWithContext(ctx context.Context, operation string, fn func(context.Context) error) error {
	timer := m.NewTimer(operation)
	defer timer.Stop()
	return fn(ctx)
}
