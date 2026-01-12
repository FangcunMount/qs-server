package cache

import "strings"

var keyNamespace string

// ApplyNamespace 设置全局 Redis key 命名空间（可选），会自动在前面加上 "ns:"。
// 传入空字符串表示不使用命名空间。
func ApplyNamespace(ns string) {
	keyNamespace = strings.Trim(ns, ":")
}

// addNamespace 在 key 前增加命名空间（如果设置了）
func addNamespace(key string) string {
	if keyNamespace == "" {
		return key
	}
	return keyNamespace + ":" + key
}
