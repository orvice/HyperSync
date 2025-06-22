package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SyncMetrics provides a convenient wrapper for sync-related metrics
type SyncMetrics struct {
	mainSocial string
}

// NewSyncMetrics creates a new SyncMetrics instance for the given main social platform
func NewSyncMetrics(mainSocial string) *SyncMetrics {
	return &SyncMetrics{
		mainSocial: mainSocial,
	}
}

// Timer represents a timing measurement
type Timer struct {
	histogram prometheus.Observer
	start     time.Time
}

// NewTimer creates a new timer for the given operation
func (m *SyncMetrics) NewTimer(operation string) *Timer {
	return &Timer{
		histogram: SyncOperationDuration.WithLabelValues(m.mainSocial, operation),
		start:     time.Now(),
	}
}

// Stop stops the timer and records the duration
func (t *Timer) Stop() {
	t.histogram.Observe(time.Since(t.start).Seconds())
}

// IncPostsProcessed increments the posts processed counter
func (m *SyncMetrics) IncPostsProcessed(status string) {
	SyncPostsProcessedTotal.WithLabelValues(m.mainSocial, status).Inc()
}

// IncCrossPosts increments the cross-posts counter
func (m *SyncMetrics) IncCrossPosts(targetPlatform, status string) {
	SyncCrossPostsTotal.WithLabelValues(m.mainSocial, targetPlatform, status).Inc()
}

// IncDatabaseOps increments the database operations counter
func (m *SyncMetrics) IncDatabaseOps(operation, status string) {
	SyncDatabaseOpsTotal.WithLabelValues(m.mainSocial, operation, status).Inc()
}

// IncErrors increments the errors counter
func (m *SyncMetrics) IncErrors(targetPlatform, errorType string) {
	SyncErrorsTotal.WithLabelValues(m.mainSocial, targetPlatform, errorType).Inc()
}

// IncRetries increments the retries counter
func (m *SyncMetrics) IncRetries(targetPlatform string) {
	SyncRetriesTotal.WithLabelValues(m.mainSocial, targetPlatform).Inc()
}

// SetPostsInQueue sets the posts in queue gauge
func (m *SyncMetrics) SetPostsInQueue(count int) {
	SyncPostsInQueue.WithLabelValues(m.mainSocial).Set(float64(count))
}

// IncActiveOperations increments the active operations gauge
func (m *SyncMetrics) IncActiveOperations() {
	SyncActiveOperations.WithLabelValues(m.mainSocial).Inc()
}

// DecActiveOperations decrements the active operations gauge
func (m *SyncMetrics) DecActiveOperations() {
	SyncActiveOperations.WithLabelValues(m.mainSocial).Dec()
}

// ActiveOperationsContext tracks active operations using context
func (m *SyncMetrics) ActiveOperationsContext(ctx context.Context, fn func(context.Context) error) error {
	m.IncActiveOperations()
	defer m.DecActiveOperations()
	return fn(ctx)
}

// TimedOperation executes a function and times it
func (m *SyncMetrics) TimedOperation(operation string, fn func() error) error {
	timer := m.NewTimer(operation)
	defer timer.Stop()
	return fn()
}

// TimedOperationWithContext executes a function with context and times it
func (m *SyncMetrics) TimedOperationWithContext(ctx context.Context, operation string, fn func(context.Context) error) error {
	timer := m.NewTimer(operation)
	defer timer.Stop()
	return fn(ctx)
}
