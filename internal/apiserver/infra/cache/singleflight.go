package cache

import "golang.org/x/sync/singleflight"

// Group 是缓存层使用的 singleflight 组，避免击穿时并发回源
var Group singleflight.Group
