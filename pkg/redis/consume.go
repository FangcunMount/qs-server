package rediskit

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

var consumeIfExistsScript = goredis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	redis.call("DEL", KEYS[1])
	return 1
else
	return 0
end
`)

// ConsumeIfExists atomically checks and deletes a single key.
func ConsumeIfExists(ctx context.Context, client goredis.UniversalClient, key string) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("redis client is nil")
	}
	if key == "" {
		return false, fmt.Errorf("consume key is empty")
	}

	result, err := consumeIfExistsScript.Run(ctx, client, []string{key}).Int64()
	if err == goredis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
