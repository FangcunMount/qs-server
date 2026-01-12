package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// 简单工具：为已有的 stats:* 键补充 TTL，防止历史无 TTL 键无限增长。
func main() {
	var (
		addr       string
		username   string
		password   string
		db         int
		dryRun     bool
		ttlDaily   time.Duration
		ttlWindow  time.Duration
		ttlDist    time.Duration
		ttlAccum   time.Duration
		scanCount  int64
		expireDone int64
	)

	flag.StringVar(&addr, "addr", "127.0.0.1:6379", "redis address")
	flag.StringVar(&username, "user", "", "redis username (optional)")
	flag.StringVar(&password, "pass", "", "redis password (optional)")
	flag.IntVar(&db, "db", 0, "redis db index")
	flag.BoolVar(&dryRun, "dry-run", false, "dry run, only count keys without setting TTL")
	flag.DurationVar(&ttlDaily, "ttl-daily", 90*24*time.Hour, "TTL for stats:daily:* keys")
	flag.DurationVar(&ttlWindow, "ttl-window", 90*24*time.Hour, "TTL for stats:window:* keys")
	flag.DurationVar(&ttlDist, "ttl-dist", 90*24*time.Hour, "TTL for stats:dist:* keys")
	flag.DurationVar(&ttlAccum, "ttl-accum", 0, "TTL for stats:accum:* keys (0 to skip)")
	flag.Parse()

	opts := &redis.Options{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       db,
	}
	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to ping redis: %v", err)
	}

	type patternTTL struct {
		pattern string
		ttl     time.Duration
		label   string
	}
	patterns := []patternTTL{
		{pattern: "stats:daily:*", ttl: ttlDaily, label: "daily"},
		{pattern: "stats:window:*", ttl: ttlWindow, label: "window"},
		{pattern: "stats:dist:*", ttl: ttlDist, label: "dist"},
		{pattern: "stats:accum:*", ttl: ttlAccum, label: "accum"},
	}

	for _, p := range patterns {
		if p.ttl <= 0 {
			log.Printf("skip pattern %s (ttl<=0)\n", p.pattern)
			continue
		}
		log.Printf("processing pattern=%s ttl=%s\n", p.pattern, p.ttl)
		cursor := uint64(0)
		for {
			keys, nextCursor, err := client.Scan(ctx, cursor, p.pattern, 500).Result()
			if err != nil {
				log.Fatalf("scan failed on pattern %s: %v", p.pattern, err)
			}
			scanCount += int64(len(keys))
			if !dryRun && len(keys) > 0 {
				for _, k := range keys {
					if err := client.Expire(ctx, k, p.ttl).Err(); err != nil {
						log.Fatalf("set ttl failed for key=%s: %v", k, err)
					}
					expireDone++
				}
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}

	fmt.Printf("scan keys=%d, ttl set=%d, dryRun=%v\n", scanCount, expireDone, dryRun)
}
