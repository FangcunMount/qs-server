package rediskit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

var releaseLeaseScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`)

// AcquireLease tries to acquire a leased lock using SET NX EX semantics.
func AcquireLease(ctx context.Context, client goredis.UniversalClient, key string, ttl time.Duration) (string, bool, error) {
	if client == nil {
		return "", false, fmt.Errorf("redis client is nil")
	}
	if key == "" {
		return "", false, fmt.Errorf("lock key is empty")
	}
	if ttl <= 0 {
		return "", false, fmt.Errorf("lock ttl must be positive")
	}

	token := uuid.NewString()
	ok, err := client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

// ReleaseLease releases a lock only when the lease token matches.
func ReleaseLease(ctx context.Context, client goredis.UniversalClient, key, token string) error {
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}
	if key == "" || token == "" {
		return nil
	}

	_, err := releaseLeaseScript.Run(ctx, client, []string{key}, token).Result()
	if err == goredis.Nil {
		return nil
	}
	return err
}
