package cache

import "github.com/FangcunMount/qs-server/internal/pkg/rediskey"

// ApplyNamespace 设置全局 Redis key 命名空间（可选）。
func ApplyNamespace(ns string) {
	rediskey.ApplyNamespace(ns)
}

// addNamespace 在 key 前增加命名空间（如果设置了）。
func addNamespace(key string) string {
	return rediskey.AddNamespace(key)
}
