package definition

import (
	"context"
	"encoding/json"
	"fmt"

	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Materialization describes the validated runtime identity derived from DefinitionV2.
// It deliberately contains no compatibility payload bytes or format labels.
type Materialization struct {
	Kind            domain.Kind
	SubKind         domain.SubKind
	Algorithm       domain.Algorithm
	AlgorithmFamily domain.AlgorithmFamily
	DecisionKind    domain.DecisionKind
	Version         string
}

// Handler 拥有家庭特定的定义验证和发布塑造
// Family handlers 验证规范的 DefinitionV2 时提供；遗留的导入属于所属的 API 适配器边界。
type Handler interface {
	// Supports reports whether this handler owns the AlgorithmBinding
	// represented by identity (Kind + SubKind + Algorithm matrix).
	Supports(identity domain.Identity) bool
	// ValidateForPublish 验证发布
	ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue
	// MaterializeSnapshot validates that DefinitionV2 can build the family runtime DTO
	// and returns the complete frozen routing identity.
	MaterializeSnapshot(ctx context.Context, model *domain.AssessmentModel) (Materialization, error)
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

// Registry resolves family handlers by AlgorithmBinding (compatibility matrix).
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

// ResolveBinding resolves a handler by full AlgorithmBinding.
func (r Registry) ResolveBinding(binding AlgorithmBinding) (Handler, bool) {
	binding = binding.WithDerivedFamily()
	if !binding.Compatible() {
		return nil, false
	}
	identity := binding.Identity()
	for _, handler := range r.handlers {
		if handler.Supports(identity) {
			return handler, true
		}
	}
	return nil, false
}

// MustResolveBinding must resolve a handler by AlgorithmBinding.
func (r Registry) MustResolveBinding(binding AlgorithmBinding) (Handler, error) {
	handler, ok := r.ResolveBinding(binding)
	if ok {
		return handler, nil
	}
	return nil, fmt.Errorf(
		"unsupported assessment model algorithm binding %s/%s/%s",
		binding.Kind, binding.SubKind, binding.Algorithm,
	)
}

// Resolve resolves by Identity fields (delegates to ResolveBinding).
func (r Registry) Resolve(identity domain.Identity) (Handler, bool) {
	return r.ResolveBinding(AlgorithmBindingFromIdentity(identity))
}

// MustResolve must resolve by Identity fields (delegates to MustResolveBinding).
func (r Registry) MustResolve(identity domain.Identity) (Handler, error) {
	return r.MustResolveBinding(AlgorithmBindingFromIdentity(identity))
}

// PreviewReport 预览报告
func (r Registry) PreviewReport(ctx context.Context, model *domain.AssessmentModel, input json.RawMessage) (*PreviewResult, error) {
	if model == nil {
		return nil, fmt.Errorf("assessment model is nil")
	}
	handler, err := r.MustResolveBinding(AlgorithmBindingFromModel(model))
	if err != nil {
		return nil, err
	}
	preview, ok := handler.(PreviewHandler)
	if !ok {
		return nil, fmt.Errorf("report preview is not configured for model identity %s/%s", model.Kind, model.Algorithm)
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
