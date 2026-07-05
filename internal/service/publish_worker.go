package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.orx.me/apps/hyper-sync/internal/media"
	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// syncPostTimeout bounds one post's platform calls so a hung platform cannot
// stall the single worker goroutine forever.
const syncPostTimeout = 2 * time.Minute

type PublishWorker struct {
	store      post.Store
	mediaStore media.Store
	clients    map[string]social.SocialClient
	maxRetries int
}

func NewPublishWorker(store post.Store, mediaStore media.Store, clients map[string]social.SocialClient, maxRetries int) *PublishWorker {
	return &PublishWorker{
		store:      store,
		mediaStore: mediaStore,
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
			// Not syncable — clear the flag so it isn't refetched every tick.
			if err := w.store.SetSyncPending(ctx, p.ID, false); err != nil {
				slog.Warn("failed to clear sync_pending", "post_id", p.ID, "error", err)
			}
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, syncPostTimeout)
		w.syncPost(pctx, p)
		cancel()
	}

	return nil
}

func shouldSync(visibility string) bool {
	return visibility == "public" || visibility == "unlisted"
}

func (w *PublishWorker) syncPost(ctx context.Context, p *post.Post) {
	socialPost := &social.Post{
		Content:    p.Content,
		Visibility: toVisibilityLevel(p.Visibility),
		Media:      w.buildMedia(ctx, p.MediaIDs),
	}

	for _, target := range p.SyncTargets {
		status := p.CrossPostStatus[target]

		client, ok := w.clients[target]
		if !ok {
			slog.Warn("no client configured for platform", "platform", target, "post_id", p.ID)
			continue
		}

		if status.NeedsUpdate && status.PlatformID != "" {
			// Update path: content changed after initial sync
			if status.RetryCount >= w.maxRetries {
				continue
			}
			if updater, ok := client.(social.SocialUpdater); ok {
				err := updater.Update(ctx, status.PlatformID, socialPost)
				if err != nil {
					status.Error = err.Error()
					status.RetryCount++
					slog.Error("update on platform failed", "platform", target, "post_id", p.ID, "retry", status.RetryCount, "error", err)
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
			w.persistStatus(ctx, p.ID, target, status)
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
			status.PlatformID = social.ExtractPlatformID(result)
			if status.PlatformID == "" {
				slog.Warn("could not extract platform id, update/delete will not work for this post", "platform", target, "post_id", p.ID)
			}
			slog.Info("synced post to platform", "platform", target, "post_id", p.ID, "platform_id", status.PlatformID)
		}

		p.CrossPostStatus[target] = status
		w.persistStatus(ctx, p.ID, target, status)
	}

	// Recompute the pending flag from the fresh local state so fully synced
	// (or retry-exhausted) posts drop out of the next tick's query.
	pending := post.ComputeSyncPending(p, w.maxRetries)
	if err := w.store.SetSyncPending(ctx, p.ID, pending); err != nil {
		slog.Warn("failed to update sync_pending", "post_id", p.ID, "error", err)
	}
}

// persistStatus writes one platform's status with a field-level update so the
// worker never overwrites concurrent user edits to the rest of the post.
func (w *PublishWorker) persistStatus(ctx context.Context, postID, platform string, status post.CrossPostStatus) {
	if err := w.store.UpdateSyncStatus(ctx, postID, platform, status); err != nil {
		slog.Error("failed to persist sync status", "post_id", postID, "platform", platform, "error", err)
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

// buildMedia resolves media IDs to CDN URLs; platforms fetch the bytes lazily
// via Media.GetData when they upload.
func (w *PublishWorker) buildMedia(ctx context.Context, mediaIDs []string) []social.Media {
	if w.mediaStore == nil || len(mediaIDs) == 0 {
		return nil
	}
	out := make([]social.Media, 0, len(mediaIDs))
	for _, id := range mediaIDs {
		m, err := w.mediaStore.GetByID(ctx, id)
		if err != nil {
			slog.Warn("failed to resolve media for sync", "media_id", id, "error", err)
			continue
		}
		if m.CDNUrl == "" {
			continue
		}
		out = append(out, *social.NewMediaFromURL(m.CDNUrl))
	}
	return out
}
