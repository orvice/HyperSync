package dao

import (
	"fmt"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/media"
)

// NewObjectStorage builds the configured S3-compatible object storage, or an
// in-memory fallback if no S3 config is present. It fails fast if an S3
// config is present but missing required fields, rather than silently
// falling back or constructing a client that will error on every upload.
func NewObjectStorage() (media.ObjectStorage, error) {
	if conf.Conf.Storage != nil && conf.Conf.Storage.S3 != nil {
		s3Conf := conf.Conf.Storage.S3
		if s3Conf.Endpoint == "" || s3Conf.Bucket == "" {
			return nil, fmt.Errorf("dao: incomplete S3 storage config: endpoint and bucket are required")
		}
		return media.NewS3ObjectStorage(media.S3Config{
			Endpoint:  s3Conf.Endpoint,
			Bucket:    s3Conf.Bucket,
			AccessKey: s3Conf.AccessKey,
			SecretKey: s3Conf.SecretKey,
			Region:    s3Conf.Region,
		}), nil
	}
	return media.NewMemoryObjectStorage(), nil
}
