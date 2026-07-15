package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	AttrClient    = "client"
	AttrMediaType = "media_type"
)

var (
	TelegramUpdatesTotal = mustInt64Counter(
		"telegram_updates_received_total",
		"Total number of Telegram updates received",
	)
	TelegramPostsBufferedTotal = mustInt64Counter(
		"telegram_posts_buffered_total",
		"Total number of posts buffered from Telegram",
	)
	TelegramMediaUploadsTotal = mustInt64Counter(
		"telegram_media_uploads_total",
		"Total number of Telegram media upload attempts",
	)
	TelegramMediaUploadDuration = mustFloat64Histogram(
		"telegram_media_upload_duration_seconds",
		"Duration of Telegram media download+upload operations",
		"s",
	)
	TelegramBufferSize = mustInt64Gauge(
		"telegram_buffer_size",
		"Current number of posts in the Telegram buffer",
	)
)

// TelegramMetrics provides convenience methods for Telegram instrumentation.
type TelegramMetrics struct {
	client attribute.KeyValue
}

func NewTelegramMetrics(clientName string) *TelegramMetrics {
	return &TelegramMetrics{
		client: attribute.String(AttrClient, clientName),
	}
}

func (m *TelegramMetrics) IncUpdates() {
	TelegramUpdatesTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.client))
}

func (m *TelegramMetrics) IncPostsBuffered() {
	TelegramPostsBufferedTotal.Add(context.Background(), 1,
		metric.WithAttributes(m.client))
}

func (m *TelegramMetrics) RecordMediaUpload(mediaType, status string, duration time.Duration) {
	attrs := metric.WithAttributes(
		m.client,
		attribute.String(AttrMediaType, mediaType),
		attribute.String(AttrStatus, status),
	)
	TelegramMediaUploadsTotal.Add(context.Background(), 1, attrs)

	if status == StatusSuccess {
		TelegramMediaUploadDuration.Record(context.Background(), duration.Seconds(),
			metric.WithAttributes(m.client, attribute.String(AttrMediaType, mediaType)))
	}
}

func (m *TelegramMetrics) SetBufferSize(size int) {
	TelegramBufferSize.Record(context.Background(), int64(size),
		metric.WithAttributes(m.client))
}
