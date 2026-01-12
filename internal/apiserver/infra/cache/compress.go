package cache

import (
	"bytes"
	"compress/gzip"
	"io"
)

// EnableCompression 控制是否对缓存值进行 gzip 压缩
var EnableCompression bool

// ApplyCompressionFlag 在启动时配置全局压缩开关
func ApplyCompressionFlag(enable bool) {
	EnableCompression = enable
}

// compressIfEnabled gzip 压缩（可选）
func compressIfEnabled(data []byte) []byte {
	if !EnableCompression || len(data) == 0 {
		return data
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return data
	}
	_ = w.Close()
	return buf.Bytes()
}

// decompressIfNeeded 尝试解压 gzip，不是 gzip 时返回原数据
func decompressIfNeeded(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return data
	}
	return out
}
