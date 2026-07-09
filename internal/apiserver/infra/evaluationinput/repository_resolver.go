package evaluationinput

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type RepositoryResolver struct {
	scaleCatalog    port.ScaleCatalog
	typologyCatalog port.TypologyModelCatalog
	providers       *ModelInputProviderRegistry
}

// NewRepositoryResolver builds evaluation input resolution from survey repositories
// and the published assessment model catalog.
func NewRepositoryResolver(
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	modelCatalog rulesetport.Catalog,
	descs []evaldomain.ModelDescriptor,
) (*RepositoryResolver, error) {
	if len(descs) == 0 {
		return nil, fmt.Errorf("evaluation model descriptors are required")
	}
	if modelCatalog == nil {
		return nil, fmt.Errorf("ruleset catalog is required")
	}
	interpretationScaleCatalog := NewPublishedScaleCatalog(modelCatalog, nil)
	var (
		typologyCatalog         port.TypologyModelCatalog
		behavioralRatingCatalog port.BehavioralRatingModelCatalog
		cognitiveCatalog        port.CognitiveModelCatalog
	)
	if publishedReader, ok := modelCatalog.(rulesetport.PublishedModelReader); ok {
		typologyCatalog = NewPublishedTypologyCatalog(publishedReader)
		behavioralRatingCatalog = NewPublishedBehavioralRatingCatalog(publishedReader)
		cognitiveCatalog = NewPublishedCognitiveCatalog(publishedReader)
	} else {
		return nil, fmt.Errorf("ruleset catalog must implement PublishedModelReader")
	}
	answerSheetReader := NewRepositoryAnswerSheetSnapshotReader(answerSheetRepo)
	questionnaireReader := NewRepositoryQuestionnaireSnapshotReader(questionnaireRepo)
	providers, err := MaterializeInputProviders(descs, InputProviderDeps{
		ScaleCatalog:            interpretationScaleCatalog,
		TypologyCatalog:         typologyCatalog,
		BehavioralRatingCatalog: behavioralRatingCatalog,
		CognitiveCatalog:        cognitiveCatalog,
		AnswerSheets:            answerSheetReader,
		Questionnaires:          questionnaireReader,
	})
	if err != nil {
		return nil, err
	}
	return newResolver(interpretationScaleCatalog, typologyCatalog, providers...)
}

func NewResolver(
	scaleCatalog port.ScaleModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	return newResolver(scaleCatalog, nil, providers...)
}

func newResolver(
	scaleCatalog port.ScaleModelCatalog,
	typologyCatalog port.TypologyModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	providerRegistry, err := NewModelInputProviderRegistry(providers...)
	if err != nil {
		return nil, err
	}
	return &RepositoryResolver{
		scaleCatalog:    scaleCatalog,
		typologyCatalog: typologyCatalog,
		providers:       providerRegistry,
	}, nil
}

func (r *RepositoryResolver) Resolve(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	modelRef := normalizeModelRef(ref)
	provider, err := r.providers.Resolve(modelRef.ExecutionIdentity())
	if err != nil {
		err := fmt.Errorf("unsupported evaluation model key: %s", modelRef.ExecutionIdentity())
		return nil, port.NewResolveError(port.FailureKindUnsupportedModel, err, "不支持的解释模型", "加载解释模型失败")
	}
	ref.ModelRef = modelRef
	return provider.ResolveInput(ctx, ref)
}

func normalizeModelRef(ref port.InputRef) port.ModelRef {
	if !ref.ModelRef.IsEmpty() {
		return ref.ModelRef
	}
	if ref.MedicalScaleCode != "" {
		return port.ModelRef{Kind: port.EvaluationModelKindScale, Code: ref.MedicalScaleCode}
	}
	return port.ModelRef{}
}

func (r *RepositoryResolver) GetScale(ctx context.Context, code string) (*scalesnapshot.ScaleSnapshot, error) {
	return r.scaleCatalog.GetScale(ctx, code)
}

func (r *RepositoryResolver) FindTypologyModelByQuestionnaire(ctx context.Context, code, version string) (*modeltypology.Payload, error) {
	if r == nil || r.typologyCatalog == nil {
		return nil, fmt.Errorf("typology model catalog is not configured")
	}
	return r.typologyCatalog.FindTypologyModelByQuestionnaire(ctx, code, version)
}

type ModelInputProvider interface {
	ExecutionIdentity() evaldomain.ExecutionIdentity
	ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error)
}

type ModelInputProviderRegistry struct {
	items map[evaldomain.ExecutionIdentity]ModelInputProvider
}

func NewModelInputProviderRegistry(providers ...ModelInputProvider) (*ModelInputProviderRegistry, error) {
	registry := &ModelInputProviderRegistry{items: make(map[evaldomain.ExecutionIdentity]ModelInputProvider)}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *ModelInputProviderRegistry) Register(provider ModelInputProvider) error {
	if provider == nil {
		return fmt.Errorf("evaluation input provider is nil")
	}
	key := provider.ExecutionIdentity()
	if key.IsZero() {
		return fmt.Errorf("evaluation input provider key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation input provider already registered for key %s", key)
	}
	r.items[key] = provider
	return nil
}

func (r *ModelInputProviderRegistry) Resolve(key evaldomain.ExecutionIdentity) (ModelInputProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation input provider registry is not configured")
	}
	if provider, ok := r.items[key]; ok {
		return provider, nil
	}
	if routed := evaldomain.ResolvePersonalityTypologyExecutorIdentity(key); routed != key {
		if provider, ok := r.items[routed]; ok {
			return provider, nil
		}
	}
	if routed := evaldomain.ResolveBehavioralRatingExecutorIdentity(key); routed != key {
		if provider, ok := r.items[routed]; ok {
			return provider, nil
		}
	}
	return nil, fmt.Errorf("unsupported evaluation model key: %s", key)
}

type ScaleModelInputProvider struct {
	scaleCatalog        port.ScaleModelCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

func NewScaleModelInputProvider(
	scaleCatalog port.ScaleModelCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) ScaleModelInputProvider {
	return ScaleModelInputProvider{
		scaleCatalog:        scaleCatalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (ScaleModelInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityScaleDefault
}

func (ScaleModelInputProvider) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathScaleDescriptor
}

func (p ScaleModelInputProvider) ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	medicalScale, err := p.scaleCatalog.GetScaleByRef(ctx, ref.ModelRef)
	if err != nil {
		return nil, err
	}
	answerSheet, err := p.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	qnr, err := p.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}

	payload := port.ScaleModelPayload{Scale: medicalScale}
	return &port.InputSnapshot{
		Model:         port.NewScaleModelSnapshot(medicalScale),
		ModelPayload:  payload,
		MedicalScale:  medicalScale,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}

type RepositoryScaleSnapshotCatalog struct {
	repo ScaleSnapshotRepository
}

// ScaleSnapshotRepository is the narrow Scale read port needed by evaluation
// input resolution. Command repositories may implement it, but providers should
// not depend on Scale mutation capabilities.
type ScaleSnapshotRepository interface {
	FindByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	FindByCodeVersion(ctx context.Context, code, scaleVersion string) (*scaledefinition.MedicalScale, error)
	FindByQuestionnaireRef(ctx context.Context, questionnaireCode, questionnaireVersion string) (*scaledefinition.MedicalScale, error)
}

type publishedScaleSnapshotRepository interface {
	FindPublishedByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
}

func NewRepositoryScaleSnapshotCatalog(repo ScaleSnapshotRepository) *RepositoryScaleSnapshotCatalog {
	return &RepositoryScaleSnapshotCatalog{repo: repo}
}

func (r *RepositoryScaleSnapshotCatalog) GetScale(ctx context.Context, code string) (*scalesnapshot.ScaleSnapshot, error) {
	l := logger.L(ctx)
	l.Debugw("加载量表数据",
		"scale_code", code,
		"action", "read",
		"resource", "scale",
	)

	medicalScale, err := r.findCurrentPublishedScale(ctx, code)
	if err != nil {
		l.Errorw("加载量表失败",
			"scale_code", code,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, port.NewResolveError(port.FailureKindScaleNotFound, err, "量表不存在", "加载量表失败")
	}

	l.Debugw("量表数据加载成功",
		"scale_code", code,
		"scale_title", medicalScale.GetTitle(),
		"result", "success",
	)
	return scaleToSnapshot(medicalScale), nil
}

func (r *RepositoryScaleSnapshotCatalog) GetScaleByRef(ctx context.Context, ref port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
	l := logger.L(ctx)
	l.Debugw("加载解释模型数据",
		"model_kind", ref.Kind,
		"ruleset_kind", ref.Kind,
		"model_code", ref.Code,
		"ruleset_version", ref.Version,
		"action", "read",
		"resource", "scale",
	)

	var (
		medicalScale *scaledefinition.MedicalScale
		err          error
	)
	if ref.Version != "" {
		medicalScale, err = r.repo.FindByCodeVersion(ctx, ref.Code, ref.Version)
	} else {
		medicalScale, err = r.findCurrentPublishedScale(ctx, ref.Code)
	}
	if err != nil {
		l.Errorw("加载解释模型失败",
			"model_kind", ref.Kind,
			"ruleset_kind", ref.Kind,
			"model_code", ref.Code,
			"ruleset_version", ref.Version,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型不存在", "加载解释模型失败")
	}
	snapshot := scaleToSnapshot(medicalScale)
	if snapshot == nil || (ref.Version != "" && snapshot.ScaleVersion != ref.Version) {
		err := fmt.Errorf("解释模型版本不存在或不匹配")
		return nil, port.NewResolveError(port.FailureKindModelNotFound, err, "解释模型版本不存在或不匹配", "加载解释模型失败")
	}

	l.Debugw("解释模型数据加载成功",
		"model_kind", ref.Kind,
		"ruleset_kind", ref.Kind,
		"model_code", ref.Code,
		"ruleset_version", snapshot.ScaleVersion,
		"result", "success",
	)
	return snapshot, nil
}

func (r *RepositoryScaleSnapshotCatalog) findCurrentPublishedScale(ctx context.Context, code string) (*scaledefinition.MedicalScale, error) {
	if repo, ok := r.repo.(publishedScaleSnapshotRepository); ok {
		return repo.FindPublishedByCode(ctx, code)
	}
	return r.repo.FindByCode(ctx, code)
}

type RepositoryAnswerSheetSnapshotReader struct {
	repo answersheet.Repository
}

func NewRepositoryAnswerSheetSnapshotReader(repo answersheet.Repository) *RepositoryAnswerSheetSnapshotReader {
	return &RepositoryAnswerSheetSnapshotReader{repo: repo}
}

func (r *RepositoryAnswerSheetSnapshotReader) GetAnswerSheet(ctx context.Context, answerSheetID uint64) (*port.AnswerSheetSnapshot, error) {
	l := logger.L(ctx)
	l.Debugw("加载答卷数据",
		"answer_sheet_id", answerSheetID,
		"action", "read",
		"resource", "answersheet",
	)

	answerSheet, err := r.repo.FindByID(ctx, meta.FromUint64(answerSheetID))
	if err != nil {
		l.Errorw("加载答卷失败",
			"answer_sheet_id", answerSheetID,
			"action", "evaluate_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, port.NewResolveError(port.FailureKindAnswerSheetNotFound, err, "答卷不存在", "加载答卷失败")
	}

	snapshot := answerSheetToSnapshot(answerSheet)
	l.Debugw("答卷数据加载成功",
		"answer_sheet_id", answerSheetID,
		"questionnaire_code", snapshot.QuestionnaireCode,
		"result", "success",
	)
	return snapshot, nil
}

type RepositoryQuestionnaireSnapshotReader struct {
	repo questionnaire.Repository
}

func NewRepositoryQuestionnaireSnapshotReader(repo questionnaire.Repository) *RepositoryQuestionnaireSnapshotReader {
	return &RepositoryQuestionnaireSnapshotReader{repo: repo}
}

func (r *RepositoryQuestionnaireSnapshotReader) GetQuestionnaire(ctx context.Context, code, version string) (*port.QuestionnaireSnapshot, error) {
	l := logger.L(ctx)
	l.Debugw("加载问卷数据",
		"questionnaire_code", code,
		"questionnaire_version", version,
		"action", "read",
		"resource", "questionnaire",
	)

	qnr, err := r.repo.FindByCodeVersion(ctx, code, version)
	if err != nil {
		l.Errorw("加载问卷失败，评估终止",
			"questionnaire_code", code,
			"questionnaire_version", version,
			"error", err.Error(),
		)
		return nil, port.NewResolveError(port.FailureKindQuestionnaireNotFound, err, "加载问卷失败", "加载问卷失败")
	}
	if qnr == nil {
		err = fmt.Errorf("问卷不存在或版本不匹配")
		l.Errorw("加载问卷失败，未命中答卷要求的精确版本",
			"questionnaire_code", code,
			"questionnaire_version", version,
			"error", err.Error(),
		)
		return nil, port.NewResolveError(port.FailureKindQuestionnaireVersionMismatch, err, "问卷不存在或版本不匹配", "加载问卷失败")
	}

	l.Debugw("问卷数据加载成功",
		"questionnaire_code", code,
		"questionnaire_version", version,
		"question_count", len(qnr.GetQuestions()),
		"result", "success",
	)
	return questionnaireToSnapshot(qnr), nil
}

func FailureReason(err error) string {
	var carrier port.FailureReasonCarrier
	if stderrors.As(err, &carrier) {
		return carrier.FailureReason()
	}
	return "评估输入加载失败: " + err.Error()
}
