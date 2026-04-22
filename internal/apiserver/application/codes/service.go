package codes

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// codesService 基于 internal/pkg/meta 的简单实现。
type codesService struct{}

// NewService 创建 CodesService 实例
func NewService() CodesService {
	return &codesService{}
}

// Apply 实现 CodesService.Apply
func (s *codesService) Apply(_ context.Context, kind string, count int, prefix string, _ map[string]interface{}) ([]string, error) {
	if count <= 0 {
		count = 1
	}

	codes := make([]string, 0, count)

	// 使用 internal/pkg/meta 提供的生成器
	// 规范化 kind 为固定短类型：qs, qu, fa
	var mappedKind string
	switch kind {
	case "qs", "questionnaire", "questionnaire_code":
		mappedKind = "qs"
	case "qu", "question", "question_code":
		mappedKind = "qu"
	case "fa", "factor", "factor_code":
		mappedKind = "fa"
	default:
		// 如果传入未知 kind，则直接使用原始值（但去掉特殊字符）
		mappedKind = kind
	}

	// 如果没有显式 prefix，则使用 mappedKind- 作为前缀
	fullPrefix := prefix
	if fullPrefix == "" {
		fullPrefix = mappedKind + "-"
	} else {
		if fullPrefix[len(fullPrefix)-1] != '-' {
			fullPrefix = fullPrefix + "-"
		}
	}

	for i := 0; i < count; i++ {
		c, err := meta.GenerateCodeWithPrefix(fullPrefix)
		if err != nil {
			return nil, err
		}
		codes = append(codes, c.String())
	}

	return codes, nil
}
