package codes

import "context"

// CodesService 提供 code 申请的应用层接口
type CodesService interface {
	// Apply 申请指定类型的唯一 code
	Apply(ctx context.Context, kind string, count int, prefix string, metadata map[string]interface{}) ([]string, error)
}
