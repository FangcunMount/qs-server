package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/google/uuid"
)

// entryGenerator 入口生成器实现
// 负责生成测评入口（token、URL）
type entryGenerator struct {
	baseURL string // 测评入口的基础URL（例如：https://collect.yangshujie.com/entry）
}

// NewEntryGenerator 创建入口生成器
func NewEntryGenerator(baseURL string) plan.EntryGenerator {
	return &entryGenerator{
		baseURL: baseURL,
	}
}

// GenerateEntry 生成测评入口
func (g *entryGenerator) GenerateEntry(ctx context.Context, task *planDomain.AssessmentTask) (token string, url string, expireAt time.Time, err error) {
	// 1. 生成唯一令牌
	token = uuid.New().String()

	// 2. 生成入口URL
	// 格式：{baseURL}?token={token}&task_id={task_id}
	url = fmt.Sprintf("%s?token=%s&task_id=%s", g.baseURL, token, task.GetID().String())

	// 3. 计算过期时间（默认从开放时间起 7 天）
	// 这里可以根据业务需求调整过期时间策略
	expireAt = time.Now().Add(7 * 24 * time.Hour)

	return token, url, expireAt, nil
}
