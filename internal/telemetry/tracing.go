package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Service name for tracing
	ServiceName = "hypersync"

	// Tracer name for sync operations
	SyncTracerName = "hypersync-sync"
)

// SyncTracer wraps OpenTelemetry tracer for sync operations
type SyncTracer struct {
	tracer     trace.Tracer
	mainSocial string
}

// NewSyncTracer creates a new SyncTracer instance
func NewSyncTracer(mainSocial string) *SyncTracer {
	tracer := otel.Tracer(SyncTracerName)
	return &SyncTracer{
		tracer:     tracer,
		mainSocial: mainSocial,
	}
}

// Common attribute keys
const (
	AttrMainSocial     = "hypersync.main_social"
	AttrTargetPlatform = "hypersync.target_platform"
	AttrPostID         = "hypersync.post.id"
	AttrPostContent    = "hypersync.post.content_preview"
	AttrPostVisibility = "hypersync.post.visibility"
	AttrOperation      = "hypersync.operation"
	AttrStatus         = "hypersync.status"
	AttrErrorType      = "hypersync.error.type"
	AttrPostsCount     = "hypersync.posts.count"
	AttrPlatformID     = "hypersync.platform.id"
	AttrDatabaseOp     = "hypersync.database.operation"
)

// Common attribute values
const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusSkipped   = "skipped"
	StatusExists    = "exists"
	StatusProcessed = "processed"

	OperationSync           = "sync"
	OperationFetchPosts     = "fetch_posts"
	OperationProcessPost    = "process_post"
	OperationCrossPost      = "cross_post"
	OperationDatabaseGet    = "database_get"
	OperationDatabaseCreate = "database_create"
	OperationDatabaseUpdate = "database_update"
)

// StartSyncOperation starts a new trace for the entire sync operation
func (st *SyncTracer) StartSyncOperation(ctx context.Context) (context.Context, trace.Span) {
	ctx, span := st.tracer.Start(ctx, "sync_operation",
		trace.WithAttributes(
			attribute.String(AttrMainSocial, st.mainSocial),
			attribute.String(AttrOperation, OperationSync),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)

	return ctx, span
}

// StartFetchPosts starts a span for fetching posts from the main platform
func (st *SyncTracer) StartFetchPosts(ctx context.Context, limit int) (context.Context, trace.Span) {
	ctx, span := st.tracer.Start(ctx, "fetch_posts",
		trace.WithAttributes(
			attribute.String(AttrMainSocial, st.mainSocial),
			attribute.String(AttrOperation, OperationFetchPosts),
			attribute.Int("limit", limit),
		),
	)

	return ctx, span
}

// StartProcessPost starts a span for processing a single post
func (st *SyncTracer) StartProcessPost(ctx context.Context, postID, contentPreview string) (context.Context, trace.Span) {
	ctx, span := st.tracer.Start(ctx, "process_post",
		trace.WithAttributes(
			attribute.String(AttrMainSocial, st.mainSocial),
			attribute.String(AttrOperation, OperationProcessPost),
			attribute.String(AttrPostID, postID),
			attribute.String(AttrPostContent, contentPreview),
		),
	)

	return ctx, span
}

// StartCrossPost starts a span for cross-posting to a target platform
func (st *SyncTracer) StartCrossPost(ctx context.Context, postID, targetPlatform string) (context.Context, trace.Span) {
	ctx, span := st.tracer.Start(ctx, "cross_post",
		trace.WithAttributes(
			attribute.String(AttrMainSocial, st.mainSocial),
			attribute.String(AttrTargetPlatform, targetPlatform),
			attribute.String(AttrPostID, postID),
			attribute.String(AttrOperation, OperationCrossPost),
		),
	)

	return ctx, span
}

// StartDatabaseOperation starts a span for database operations
func (st *SyncTracer) StartDatabaseOperation(ctx context.Context, operation, postID string) (context.Context, trace.Span) {
	ctx, span := st.tracer.Start(ctx, fmt.Sprintf("database_%s", operation),
		trace.WithAttributes(
			attribute.String(AttrMainSocial, st.mainSocial),
			attribute.String(AttrDatabaseOp, operation),
			attribute.String(AttrPostID, postID),
		),
	)

	return ctx, span
}

// SetSpanAttributes sets common attributes on a span
func (st *SyncTracer) SetSpanAttributes(span trace.Span, attrs map[string]interface{}) {
	for key, value := range attrs {
		switch v := value.(type) {
		case string:
			span.SetAttributes(attribute.String(key, v))
		case int:
			span.SetAttributes(attribute.Int(key, v))
		case int64:
			span.SetAttributes(attribute.Int64(key, v))
		case bool:
			span.SetAttributes(attribute.Bool(key, v))
		case float64:
			span.SetAttributes(attribute.Float64(key, v))
		default:
			span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", v)))
		}
	}
}

// SetSpanSuccess marks a span as successful with optional attributes
func (st *SyncTracer) SetSpanSuccess(span trace.Span, attrs map[string]interface{}) {
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(attribute.String(AttrStatus, StatusSuccess))
	if attrs != nil {
		st.SetSpanAttributes(span, attrs)
	}
}

// SetSpanError marks a span as failed with error information
func (st *SyncTracer) SetSpanError(span trace.Span, err error, errorType string, attrs map[string]interface{}) {
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String(AttrStatus, StatusError),
		attribute.String(AttrErrorType, errorType),
	)
	span.RecordError(err)
	if attrs != nil {
		st.SetSpanAttributes(span, attrs)
	}
}

// SetSpanSkipped marks a span as skipped with reason
func (st *SyncTracer) SetSpanSkipped(span trace.Span, reason string, attrs map[string]interface{}) {
	span.SetStatus(codes.Ok, reason)
	span.SetAttributes(attribute.String(AttrStatus, StatusSkipped))
	if attrs != nil {
		st.SetSpanAttributes(span, attrs)
	}
}

// AddEvent adds an event to the span with attributes
func (st *SyncTracer) AddEvent(span trace.Span, name string, attrs map[string]interface{}) {
	var eventAttrs []attribute.KeyValue
	for key, value := range attrs {
		switch v := value.(type) {
		case string:
			eventAttrs = append(eventAttrs, attribute.String(key, v))
		case int:
			eventAttrs = append(eventAttrs, attribute.Int(key, v))
		case int64:
			eventAttrs = append(eventAttrs, attribute.Int64(key, v))
		case bool:
			eventAttrs = append(eventAttrs, attribute.Bool(key, v))
		case float64:
			eventAttrs = append(eventAttrs, attribute.Float64(key, v))
		default:
			eventAttrs = append(eventAttrs, attribute.String(key, fmt.Sprintf("%v", v)))
		}
	}
	span.AddEvent(name, trace.WithAttributes(eventAttrs...))
}

// WithSpan executes a function within a new span
func (st *SyncTracer) WithSpan(ctx context.Context, spanName string, attrs map[string]interface{}, fn func(context.Context, trace.Span) error) error {
	ctx, span := st.tracer.Start(ctx, spanName)
	defer span.End()

	// Set initial attributes
	span.SetAttributes(attribute.String(AttrMainSocial, st.mainSocial))
	if attrs != nil {
		st.SetSpanAttributes(span, attrs)
	}

	err := fn(ctx, span)
	if err != nil {
		st.SetSpanError(span, err, "general", nil)
	} else {
		st.SetSpanSuccess(span, nil)
	}

	return err
}
