package database

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bbedward/boompow-server-ng/src/config"
	"github.com/bbedward/boompow-server-ng/src/utils"
	"github.com/go-redis/redis/v9"
)

var ctx = context.Background()

// Prefix for all keys
const keyPrefix = "boompow"

// Singleton to keep assets loaded in memory
type redisManager struct {
	Client *redis.Client
}

var singleton *redisManager
var once sync.Once

// TODO - In prod we would probably want a 3+ server redis cluster, which means these connection options would change
func GetRedisDB() *redisManager {
	once.Do(func() {

		redis_port, err := strconv.Atoi(utils.GetEnv("REDIS_PORT", "6379"))
		if err != nil {
			panic("Invalid REDIS_PORT specified")
		}
		redis_db, err := strconv.Atoi(utils.GetEnv("REDIS_DB", "0"))
		if err != nil {
			panic("Invalid REDIS_DB specified")
		}
		client := redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", utils.GetEnv("REDIS_HOST", "localhost"), redis_port),
			DB:   redis_db,
		})
		// Create locker
		// Create object
		singleton = &redisManager{
			Client: client,
		}
	})
	return singleton
}

// del - Redis DEL
func (r *redisManager) Del(key string) (int64, error) {
	val, err := r.Client.Del(ctx, key).Result()
	return val, err
}

// get - Redis GET
func (r *redisManager) Get(key string) (string, error) {
	val, err := r.Client.Get(ctx, key).Result()
	return val, err
}

// set - Redis SET
func (r *redisManager) Set(key string, value string, expiry time.Duration) error {
	err := r.Client.Set(ctx, key, value, expiry).Err()
	return err
}

// hlen - Redis HLEN
func (r *redisManager) Hlen(key string) (int64, error) {
	val, err := r.Client.HLen(ctx, key).Result()
	return val, err
}

// hget - Redis HGET
func (r *redisManager) Hget(key string, field string) (string, error) {
	val, err := r.Client.HGet(ctx, key, field).Result()
	return val, err
}

// hgetall - Redis HGETALL
func (r *redisManager) Hgetall(key string) (map[string]string, error) {
	val, err := r.Client.HGetAll(ctx, key).Result()
	return val, err
}

// hset - Redis HSET
func (r *redisManager) Hset(key string, field string, values interface{}) error {
	err := r.Client.HSet(ctx, key, field, values).Err()
	return err
}

// hdel - Redis HDEL
func (r *redisManager) Hdel(key string, field string) error {
	err := r.Client.HDel(ctx, key, field).Err()
	return err
}

// Set email confirmation token
func (r *redisManager) SetConfirmationToken(email string, token string) error {
	// Expire in 24H
	return r.Set(fmt.Sprintf("emailconfirmation:%s", email), token, config.EMAIL_CONFIRMATION_TOKEN_VALID_MINUTES*time.Minute)
}

// Get token for given email
func (r *redisManager) GetUserIDForConfirmationToken(email string) (string, error) {
	return r.Get(fmt.Sprintf("emailconfirmation:%s", email))
}

// Delete conf token
func (r *redisManager) DeleteConfirmationToken(email string) (int64, error) {
	return r.Del(fmt.Sprintf("emailconfirmation:%s", email))
}
