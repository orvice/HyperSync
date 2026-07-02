package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/social"
)

type PublishWorker struct {
	store      post.Store
	clients    map[string]social.SocialClient
	maxRetries int
}

func NewPublishWorker(store post.Store, clients map[string]social.SocialClient, maxRetries int) *PublishWorker {
	return &PublishWorker{
		store:      store,
		clients:    clients,
		maxRetries: maxRetries,
	}
}

func (w *PublishWorker) Run(ctx context.Context) error {
	posts, err := w.store.ListPendingSync(ctx)
	if err != nil {
		return fmt.Errorf("list pending sync: %w", err)
	}

	for _, p := range posts {
		if !shouldSync(p.Visibility) {
			continue
		}
		w.syncPost(ctx, p)
	}

	return nil
}

func shouldSync(visibility string) bool {
	return visibility == "public" || visibility == "unlisted"
}

func (w *PublishWorker) syncPost(ctx context.Context, p *post.Post) {
	changed := false

	for _, target := range p.SyncTargets {
		status := p.CrossPostStatus[target]

		client, ok := w.clients[target]
		if !ok {
			slog.Warn("no client configured for platform", "platform", target, "post_id", p.ID)
			continue
		}

		socialPost := &social.Post{
			Content:    p.Content,
			Visibility: toVisibilityLevel(p.Visibility),
			Media:      buildMedia(p.MediaIDs),
		}

		if status.NeedsUpdate && status.PlatformID != "" {
			// Update path: content changed after initial sync
			if updater, ok := client.(social.SocialUpdater); ok {
				err := updater.Update(ctx, status.PlatformID, socialPost)
				if err != nil {
					status.Error = err.Error()
					slog.Error("update on platform failed", "platform", target, "post_id", p.ID, "error", err)
				} else {
					status.NeedsUpdate = false
					status.Error = ""
					slog.Info("updated post on platform", "platform", target, "post_id", p.ID, "platform_id", status.PlatformID)
				}
			} else {
				status.NeedsUpdate = false
				slog.Warn("platform does not support update, clearing flag", "platform", target, "post_id", p.ID)
			}
			p.CrossPostStatus[target] = status
			changed = true
			continue
		}

		if status.Success {
			continue
		}
		if status.RetryCount >= w.maxRetries {
			continue
		}

		// Initial sync path
		result, err := client.Post(ctx, socialPost)
		now := time.Now()

		if err != nil {
			status.Error = err.Error()
			status.RetryCount++
			slog.Error("sync to platform failed", "platform", target, "post_id", p.ID, "retry", status.RetryCount, "error", err)
		} else {
			status.Success = true
			status.PostedAt = &now
			status.Error = ""
			if m, ok := result.(map[string]string); ok {
				if id, exists := m["id"]; exists {
					status.PlatformID = id
				}
			}
			slog.Info("synced post to platform", "platform", target, "post_id", p.ID, "platform_id", status.PlatformID)
		}

		p.CrossPostStatus[target] = status
		changed = true
	}

	if changed {
		if _, err := w.store.Update(ctx, p); err != nil {
			slog.Error("failed to update post sync status", "post_id", p.ID, "error", err)
		}
	}
}

func toVisibilityLevel(v string) social.VisibilityLevel {
	switch v {
	case "public":
		return social.VisibilityLevelPublic
	case "unlisted":
		return social.VisibilityLevelUnlisted
	case "private":
		return social.VisibilityLevelPrivate
	case "direct":
		return social.VisibilityLevelDirect
	default:
		return social.VisibilityLevelPublic
	}
}

func buildMedia(mediaIDs []string) []social.Media {
	// TODO: resolve media IDs to URLs for platform upload
	return nil
}
