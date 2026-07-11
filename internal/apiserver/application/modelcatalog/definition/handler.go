package definition

import (
	"context"
	"encoding/json"
	"fmt"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// SnapshotBuildResult 只包含评估模型快照所需的家庭特定部分
type SnapshotBuildResult struct {
	Kind          domain.Kind
	SubKind       domain.SubKind
	Algorithm     domain.Algorithm
	PayloadFormat string
	DecisionKind  domain.DecisionKind
	Payload       []byte
	Version       string
}

// Handler 拥有家庭特定的定义验证和发布塑造
// Family handlers 验证规范的 DefinitionV2 时提供；遗留的导入属于所属的 API 适配器边界。
type Handler interface {
	// Supports 支持特定评估模型身份
	Supports(identity domain.Identity) bool
	// ValidateForPublish 验证发布
	ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue
	// BuildSnapshotPayload 构建评估模型快照负载
	BuildSnapshotPayload(ctx context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error)
}

// PreviewResult 是策略拥有的定义报告预览表示
type PreviewResult struct {
	OutcomeCode    string             // 结果代码
	OutcomeTitle   string             // 结果标题
	ScoreDetail    map[string]float64 // 分数详情
	ReportSections []PreviewSection   // 报告部分
	RawReport      *report.Draft      // 原始报告草稿
}

// PreviewSection 是报告预览的策略部分
type PreviewSection struct {
	Title   string // 标题
	Content string // 内容
	Kind    string // 类型
}

// PreviewHandler 仅由支持报告预览的定义策略实现
type PreviewHandler interface {
	PreviewReport(context.Context, *domain.AssessmentModel, json.RawMessage) (*PreviewResult, error)
}

// Registry 通过模型身份解析家庭处理程序
type Registry struct {
	handlers []Handler
}

// NewRegistry 创建注册表
func NewRegistry(handlers ...Handler) Registry {
	copied := make([]Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			copied = append(copied, handler)
		}
	}
	return Registry{handlers: copied}
}

// Resolve 解析家庭处理程序
func (r Registry) Resolve(identity domain.Identity) (Handler, bool) {
	for _, handler := range r.handlers {
		if handler.Supports(identity) {
			return handler, true
		}
	}
	return nil, false
}

// MustResolve 必须解析家庭处理程序
func (r Registry) MustResolve(identity domain.Identity) (Handler, error) {
	handler, ok := r.Resolve(identity)
	if ok {
		return handler, nil
	}
	return nil, fmt.Errorf("unsupported assessment model identity %s/%s/%s", identity.Kind, identity.SubKind, identity.Algorithm)
}

// PreviewReport 预览报告
func (r Registry) PreviewReport(ctx context.Context, model *domain.AssessmentModel, input json.RawMessage) (*PreviewResult, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	handler, err := r.MustResolve(domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm})
	if err != nil {
		return nil, err
	}
	preview, ok := handler.(PreviewHandler)
	if !ok {
		return nil, fmt.Errorf("report preview is not configured for model identity %s/%s/%s", model.Kind, model.SubKind, model.Algorithm)
	}
	return preview.PreviewReport(ctx, model, input)
}

// ValidationError 保持结构化验证问题可见于应用编排边界
type ValidationError struct {
	Issues []domain.DomainValidationIssue // 问题
}

// NewValidationError 创建验证错误
func NewValidationError(issues []domain.DomainValidationIssue) error {
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: append([]domain.DomainValidationIssue(nil), issues...)}
}

// Error 返回验证错误信息
func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "validation failed"
	}
	return e.Issues[0].Message
}
