// Package gormutil is a util to convert offset and limit to default values.
package gormutil

// DefaultLimit 定义默认要检索的记录数。
const DefaultLimit = 1000

// LimitAndOffset 包含偏移量和限制字段。
type LimitAndOffset struct {
	Offset int
	Limit  int
}

// Unpointer 如果偏移量/限制为 nil，则填充 LimitAndOffset 的默认值，否则填充传递的值。
func Unpointer(offset *int64, limit *int64) *LimitAndOffset {
	var o, l int = 0, DefaultLimit

	if offset != nil {
		o = int(*offset)
	}

	if limit != nil {
		l = int(*limit)
	}

	return &LimitAndOffset{
		Offset: o,
		Limit:  l,
	}
}
