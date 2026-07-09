package definition

import (
	stderrors "errors"
)

// ErrNotFound 表示量表仓储未找到目标记录。
var ErrNotFound = stderrors.New("scale not found")

// IsNotFound 判断错误是否为量表仓储未找到。
func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

// HotScaleSummary 表示按填写热度聚合后的量表摘要。
type HotScaleSummary struct {
	Scale           *MedicalScale
	SubmissionCount int64
}
