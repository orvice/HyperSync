package service

import (
	"context"
	"fmt"

	"go.orx.me/apps/hyper-sync/internal/social"
)

// SocialPlatformDeleter implements PlatformDeleter using social clients.
type SocialPlatformDeleter struct {
	clients map[string]social.SocialClient
}

func NewSocialPlatformDeleter(clients map[string]social.SocialClient) *SocialPlatformDeleter {
	return &SocialPlatformDeleter{clients: clients}
}

func (d *SocialPlatformDeleter) DeleteFromPlatform(ctx context.Context, platform, platformID string) error {
	client, ok := d.clients[platform]
	if !ok {
		return fmt.Errorf("no client configured for platform %s", platform)
	}

	deleter, ok := client.(social.SocialDeleter)
	if !ok {
		return fmt.Errorf("platform %s does not support deletion", platform)
	}

	return deleter.Delete(ctx, platformID)
}
