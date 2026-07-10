package modelcatalog

import (
	"context"
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// CatalogManagementService owns model catalogue lifecycle commands.
type CatalogManagementService interface {
	Create(ctx context.Context, actor ActorContext, input CreateModelDTO) (*ModelSummary, error)
	UpdateBasicInfo(ctx context.Context, actor ActorContext, input UpdateBasicInfoDTO) (*ModelSummary, error)
	BindQuestionnaire(ctx context.Context, actor ActorContext, input BindQuestionnaireDTO) (*QuestionnaireBindingResult, error)
	Archive(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	Delete(ctx context.Context, actor ActorContext, code string) error
	SynchronizeQuestionnaireVersion(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) error
}

// DefinitionAuthoringService owns canonical DefinitionV2 editing commands.
type DefinitionAuthoringService interface {
	GetDefinition(ctx context.Context, actor ActorContext, code string) (*domain.Definition, error)
	SaveDefinition(ctx context.Context, actor ActorContext, code string, definition *domain.Definition) (*domain.Definition, error)
	ValidateDefinition(ctx context.Context, actor ActorContext, code string) (*ValidationResult, error)
	PreviewReport(ctx context.Context, actor ActorContext, code string, input json.RawMessage) (*PreviewReportResult, error)
	ApplyCodes(ctx context.Context, actor ActorContext, input ApplyCodesDTO) ([]string, error)
}

// PublicationService owns publication state transitions and snapshot creation.
type PublicationService interface {
	Publish(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	Unpublish(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
}

// CatalogQueryService owns management and published catalogue read models.
type CatalogQueryService interface {
	Get(ctx context.Context, actor ActorContext, code string) (*ModelSummary, error)
	List(ctx context.Context, actor ActorContext, input ListModelsDTO) (*ModelListResult, error)
	GetQuestionnaire(ctx context.Context, actor ActorContext, code string) (*QuestionnaireBindingResult, error)
	Options(ctx context.Context, actor ActorContext, kind string) (*OptionsResult, error)
	GetQRCode(ctx context.Context, actor ActorContext, code string) (string, error)
}

// PublishedModelResolver is the runtime-only access path for immutable models.
type PublishedModelResolver interface {
	ResolveByRef(ctx context.Context, actor ActorContext, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error)
	ResolveByQuestionnaire(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error)
}
