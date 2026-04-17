package rediskit

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

const (
	defaultScanCount       int64 = 100
	defaultDeleteBatchSize       = 500
)

// DeleteByPatternOptions controls batched key deletion.
type DeleteByPatternOptions struct {
	ScanCount int64
	BatchSize int
	UseUnlink bool
}

// ScanKeys collects keys using SCAN for the provided pattern.
func ScanKeys(ctx context.Context, client goredis.UniversalClient, pattern string, count int64) ([]string, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	if count <= 0 {
		count = defaultScanCount
	}

	keys := make([]string, 0)
	var cursor uint64
	for {
		batch, nextCursor, err := client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// DeleteByPattern scans and deletes keys in batches.
func DeleteByPattern(ctx context.Context, client goredis.UniversalClient, pattern string, opts DeleteByPatternOptions) (int, error) {
	if client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}

	opts = normalizeDeleteByPatternOptions(opts)

	var (
		cursor  uint64
		deleted int
	)
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, opts.ScanCount).Result()
		if err != nil {
			return deleted, err
		}
		for start := 0; start < len(keys); start += opts.BatchSize {
			end := start + opts.BatchSize
			if end > len(keys) {
				end = len(keys)
			}
			batch := keys[start:end]
			if len(batch) == 0 {
				continue
			}
			var count int64
			if opts.UseUnlink {
				count, err = client.Unlink(ctx, batch...).Result()
			} else {
				count, err = client.Del(ctx, batch...).Result()
			}
			if err != nil {
				return deleted, err
			}
			deleted += int(count)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return deleted, nil
}

func normalizeDeleteByPatternOptions(opts DeleteByPatternOptions) DeleteByPatternOptions {
	if opts == (DeleteByPatternOptions{}) {
		return DeleteByPatternOptions{
			ScanCount: defaultScanCount,
			BatchSize: defaultDeleteBatchSize,
			UseUnlink: true,
		}
	}
	if opts.ScanCount <= 0 {
		opts.ScanCount = defaultScanCount
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultDeleteBatchSize
	}
	return opts
}
