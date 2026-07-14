package dao

import (
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/media"
)

// NewObjectStorage builds the configured S3-compatible object storage, or an
// in-memory fallback if no S3 config is present.
func NewObjectStorage() media.ObjectStorage {
	if conf.Conf.Storage != nil && conf.Conf.Storage.S3 != nil {
		s3Conf := conf.Conf.Storage.S3
		return media.NewS3ObjectStorage(media.S3Config{
			Endpoint:  s3Conf.Endpoint,
			Bucket:    s3Conf.Bucket,
			AccessKey: s3Conf.AccessKey,
			SecretKey: s3Conf.SecretKey,
			Region:    s3Conf.Region,
		})
	}
	return media.NewMemoryObjectStorage()
}
