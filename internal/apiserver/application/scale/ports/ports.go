package ports

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/scale/shared"
)

// ScaleLifecycleService 量表生命周期服务（管理员/设计者）。
type ScaleLifecycleService interface {
	Create(ctx context.Context, dto shared.CreateScaleDTO) (*shared.ScaleResult, error)
	UpdateBasicInfo(ctx context.Context, dto shared.UpdateScaleBasicInfoDTO) (*shared.ScaleResult, error)
	UpdateQuestionnaire(ctx context.Context, dto shared.UpdateScaleQuestionnaireDTO) (*shared.ScaleResult, error)
	Publish(ctx context.Context, code string) (*shared.ScaleResult, error)
	Unpublish(ctx context.Context, code string) (*shared.ScaleResult, error)
	Archive(ctx context.Context, code string) (*shared.ScaleResult, error)
	Delete(ctx context.Context, code string) error
}

// ScaleFactorService 量表因子编辑服务。
type ScaleFactorService interface {
	AddFactor(ctx context.Context, dto shared.AddFactorDTO) (*shared.ScaleResult, error)
	UpdateFactor(ctx context.Context, dto shared.UpdateFactorDTO) (*shared.ScaleResult, error)
	RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*shared.ScaleResult, error)
	ReplaceFactors(ctx context.Context, scaleCode string, factors []shared.FactorDTO) (*shared.ScaleResult, error)
	UpdateFactorInterpretRules(ctx context.Context, dto shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error)
	ReplaceInterpretRules(ctx context.Context, scaleCode string, rules []shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error)
}

// ScaleQueryService 量表只读查询服务。
type ScaleQueryService interface {
	GetByCode(ctx context.Context, code string) (*shared.ScaleResult, error)
	GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*shared.ScaleResult, error)
	List(ctx context.Context, dto shared.ListScalesDTO) (*shared.ScaleSummaryListResult, error)
	GetPublishedByCode(ctx context.Context, code string) (*shared.ScaleResult, error)
	ListPublished(ctx context.Context, dto shared.ListScalesDTO) (*shared.ScaleSummaryListResult, error)
	ListHotPublished(ctx context.Context, dto shared.ListHotScalesDTO) (*shared.HotScaleListResult, error)
	GetFactors(ctx context.Context, scaleCode string) ([]shared.FactorResult, error)
	ResolveAssessmentScaleContext(ctx context.Context, questionnaireCode string) (*shared.AssessmentScaleContextResult, error)
}

// AssessmentScaleContextResolver 创建测评时消费的量表上下文端口。
type AssessmentScaleContextResolver interface {
	ResolveAssessmentScaleContext(ctx context.Context, questionnaireCode string) (*shared.AssessmentScaleContextResult, error)
}

// ScaleCategoryService 量表分类选项服务。
type ScaleCategoryService interface {
	GetCategories(ctx context.Context) (*shared.ScaleCategoriesResult, error)
	GetOpenCategories(ctx context.Context) (*shared.ScaleCategoriesResult, error)
}

// ScaleQRCodeGenerator 生成量表小程序码的窄端口。
type ScaleQRCodeGenerator interface {
	GenerateScaleQRCode(ctx context.Context, code string) (string, error)
}

// ScaleQRCodeQueryService 解析量表二维码展示请求。
type ScaleQRCodeQueryService interface {
	GetQRCode(ctx context.Context, code string) (string, error)
}
