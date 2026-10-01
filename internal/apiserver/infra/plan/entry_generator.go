package plan

import (
	"context"
	urlpkg "net/url"

	"github.com/FangcunMount/component-base/pkg/logger"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planentryport "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
)

// entryGenerator 入口生成器实现
// 负责生成测评入口（token、URL）
type entryGenerator struct {
	baseURL string // 测评入口的基础URL（例如：https://collect.fangcunmount.cn/entry）
}

// NewEntryGenerator 创建入口生成器
func NewEntryGenerator(baseURL string) planentryport.Generator {
	return &entryGenerator{
		baseURL: baseURL,
	}
}

// GenerateEntry 生成测评入口
func (g *entryGenerator) GenerateEntry(ctx context.Context, task *planDomain.AssessmentTask) (token string, url string, err error) {
	taskID := task.GetID().String()
	logger.L(ctx).Infow("Generating entry for task",
		"infra_action", "generate_entry",
		"task_id", taskID,
	)

	// Task IDs locate tasks; login and active profile relationships authorize access.
	token = ""
	base, parseErr := urlpkg.Parse(g.baseURL)
	if parseErr != nil {
		return "", "", parseErr
	}
	query := urlpkg.Values{}
	query.Set("task_id", taskID)
	base.RawQuery = query.Encode()
	base.Fragment = ""
	url = base.String()

	logger.L(ctx).Infow("Entry generated successfully",
		"infra_action", "generate_entry",
		"task_id", taskID,
	)

	return token, url, nil
}
