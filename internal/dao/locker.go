package dao

import (
	bredis "butterfly.orx.me/core/store/redis"
	"github.com/bsm/redislock"
	"github.com/redis/go-redis/v9"
)

func NewLocker(client *redis.Client) *redislock.Client {
	return redislock.New(client)
}

func NewRedisClient() *redis.Client {
	return bredis.GetClient("locker")
}
