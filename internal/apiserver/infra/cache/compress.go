package cache

import (
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
)

// EnableCompression 控制是否对缓存值进行 gzip 压缩。
//
// Deprecated: 仅保留给旧初始化路径；新缓存应通过 CacheCatalog / CachePolicy 控制压缩行为。
var EnableCompression bool

// ApplyCompressionFlag 在启动时配置全局压缩开关。
//
// Deprecated: 仅保留给旧初始化路径；新缓存应通过 CacheCatalog / CachePolicy 控制压缩行为。
func ApplyCompressionFlag(enable bool) {
	EnableCompression = enable
	cachepolicy.EnableCompression = enable
}

func compressData(data []byte, enable bool) []byte {
	return cachepolicy.CompressData(data, enable)
}

// compressIfEnabled gzip 压缩（可选）。
//
// Deprecated: 仅保留给旧缓存调用点；新缓存应直接使用 CachePolicy.CompressValue。
func compressIfEnabled(data []byte) []byte {
	return compressData(data, EnableCompression)
}

// decompressIfNeeded 尝试解压 gzip，不是 gzip 时返回原数据。
// 作为兼容层保留，供老缓存和“兼容旧压缩值”场景复用。
func decompressIfNeeded(data []byte) []byte {
	return cachepolicy.DecompressData(data)
}

// DecompressForCompatibility 对缓存值做向后兼容解压。
func DecompressForCompatibility(data []byte) []byte {
	return decompressIfNeeded(data)
}
