package modelcatalog

import (
	"context"
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// CatalogManagementService 拥有模型目录生命周期命令
type CatalogManagementService interface {
	Create(ctx context.Context, actor ActorContext, input CreateModelDTO) (*ModelSummary, error)
	// RestoreDraftFromPublished recreates a missing mutable draft head from an
	// active immutable snapshot. The snapshot remains active until republished.
	RestoreDraftFromPublished(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	UpdateBasicInfo(ctx context.Context, actor ActorContext, input UpdateBasicInfoDTO) (*ModelSummary, error)
	BindQuestionnaire(ctx context.Context, actor ActorContext, input BindQuestionnaireDTO) (*QuestionnaireBindingResult, error)
	Archive(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	Delete(ctx context.Context, actor ActorContext, code string) error
	SynchronizeQuestionnaireVersion(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) error
}

// DefinitionAuthoringService 拥有规范的DefinitionV2编辑命令
type DefinitionAuthoringService interface {
	GetDefinition(ctx context.Context, actor ActorContext, code string) (*domain.Definition, error)
	SaveDefinition(ctx context.Context, actor ActorContext, code string, definition *domain.Definition) (*domain.Definition, error)
	ValidateDefinition(ctx context.Context, actor ActorContext, code string) (*ValidationResult, error)
	PreviewReport(ctx context.Context, actor ActorContext, code string, input json.RawMessage) (*PreviewReportResult, error)
	ApplyCodes(ctx context.Context, actor ActorContext, input ApplyCodesDTO) ([]string, error)
}

// PublicationService 拥有发布状态过渡和快照创建
type PublicationService interface {
	Publish(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	Unpublish(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
}

// AssessmentReleaseService owns the atomic lifecycle of a questionnaire and
// assessment model pair. Standalone publication is intentionally absent.
type AssessmentReleaseService interface {
	PublishRelease(ctx context.Context, actor ActorContext, modelCode string) (*AssessmentRelease, error)
	ArchiveRelease(ctx context.Context, actor ActorContext, modelCode string) (*AssessmentRelease, error)
}

// CatalogQueryService 拥有管理和服务发布的模型目录读模型
type CatalogQueryService interface {
	Get(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	List(ctx context.Context, actor ActorContext, input ListModelsDTO) (*ModelListResult, error)
	GetPublished(ctx context.Context, actor ActorContext, code, version string) (*PublishedModelDetail, error)
	ListPublished(ctx context.Context, actor ActorContext, input ListModelsDTO) (*PublishedModelListResult, error)
	ListHotPublished(ctx context.Context, actor ActorContext, input ListModelsDTO, limit, windowDays int) (*HotModelListResult, error)
	GetQuestionnaire(ctx context.Context, actor ActorContext, code string) (*QuestionnaireBindingResult, error)
	Options(ctx context.Context, actor ActorContext, kind string) (*OptionsResult, error)
	GetQRCode(ctx context.Context, actor ActorContext, code string) (string, error)
}

// NormTableService owns immutable norm-table import and administration reads.
type NormTableService interface {
	Import(ctx context.Context, actor ActorContext, table *domain.Norm) (*NormTableDetail, error)
	Get(ctx context.Context, actor ActorContext, tableVersion string) (*NormTableDetail, error)
	List(ctx context.Context, actor ActorContext, input ListNormTablesDTO) (*NormTableListResult, error)
}

// PublishedModelResolver 是运行时只读的不可变模型访问路径
type PublishedModelResolver interface {
	ResolveByRef(ctx context.Context, actor ActorContext, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error)
	ResolveByQuestionnaire(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error)
	ResolveLatestByCode(ctx context.Context, actor ActorContext, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error)
	ListPublished(ctx context.Context, actor ActorContext, filter modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error)
}

// PublishedModelTitleResolver owns the narrow trusted-runtime lookup used by
// integrations that only need immutable display metadata.
type PublishedModelTitleResolver interface {
	ResolvePublishedTitle(ctx context.Context, kind domain.Kind, code string) (string, error)
}
