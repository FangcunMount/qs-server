package evaluationinput

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleset"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type RepositoryResolver struct {
	scaleCatalog port.ScaleCatalog
	sbtiCatalog  port.SBTIModelCatalog
	mbtiCatalog  port.MBTIModelCatalog
	providers    *ModelInputProviderRegistry
}

// NewRepositoryResolver builds the current compatibility adapter from Survey/Scale
// command repositories. New snapshot sources should implement the catalog/read-model
// ports and be wired through NewResolver instead of adding more repository/domain
// dependencies outside repository_resolver.go and snapshot_mappers.go.
func NewRepositoryResolver(
	scaleRepo ScaleSnapshotRepository,
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
	modelCatalog rulesetport.RuleSetCatalog,
) (*RepositoryResolver, error) {
	scaleCatalog := NewRepositoryScaleSnapshotCatalog(scaleRepo)
	if modelCatalog == nil {
		return nil, fmt.Errorf("ruleset catalog is required")
	}
	interpretationScaleCatalog := NewRuleSetScaleCatalog(modelCatalog, scaleCatalog)
	sbtiCatalog := NewRuleSetSBTICatalog(modelCatalog)
	mbtiCatalog := NewRuleSetMBTICatalog(modelCatalog)
	answerSheetReader := NewRepositoryAnswerSheetSnapshotReader(answerSheetRepo)
	questionnaireReader := NewRepositoryQuestionnaireSnapshotReader(questionnaireRepo)
	return NewResolverWithEmbeddedModels(
		interpretationScaleCatalog,
		sbtiCatalog,
		mbtiCatalog,
		NewScaleModelInputProvider(interpretationScaleCatalog, answerSheetReader, questionnaireReader),
		NewSBTIModelInputProvider(sbtiCatalog, answerSheetReader, questionnaireReader),
		NewMBTIModelInputProvider(mbtiCatalog, answerSheetReader, questionnaireReader),
	)
}

func NewResolver(
	scaleCatalog port.ScaleModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	return newResolver(scaleCatalog, nil, nil, providers...)
}

func NewResolverWithSBTI(
	scaleCatalog port.ScaleModelCatalog,
	sbtiCatalog port.SBTIModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	return newResolver(scaleCatalog, sbtiCatalog, nil, providers...)
}

func NewResolverWithEmbeddedModels(
	scaleCatalog port.ScaleModelCatalog,
	sbtiCatalog port.SBTIModelCatalog,
	mbtiCatalog port.MBTIModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	return newResolver(scaleCatalog, sbtiCatalog, mbtiCatalog, providers...)
}

func newResolver(
	scaleCatalog port.ScaleModelCatalog,
	sbtiCatalog port.SBTIModelCatalog,
	mbtiCatalog port.MBTIModelCatalog,
	providers ...ModelInputProvider,
) (*RepositoryResolver, error) {
	providerRegistry, err := NewModelInputProviderRegistry(providers...)
	if err != nil {
		return nil, err
	}
	return &RepositoryResolver{
		scaleCatalog: scaleCatalog,
		sbtiCatalog:  sbtiCatalog,
		mbtiCatalog:  mbtiCatalog,
		providers:    providerRegistry,
	}, nil
}

func (r *RepositoryResolver) Resolve(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	modelRef := normalizeModelRef(ref)
	provider, err := r.providers.Resolve(modelRef.Kind)
	if err != nil {
		err := fmt.Errorf("unsupported evaluation model kind: %s", modelRef.Kind)
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

func (r *RepositoryResolver) FindSBTIModelByQuestionnaire(ctx context.Context, code, version string) (*rulesetsbti.ModelSnapshot, error) {
	if r == nil || r.sbtiCatalog == nil {
		return nil, fmt.Errorf("sbti model catalog is not configured")
	}
	return r.sbtiCatalog.FindSBTIModelByQuestionnaire(ctx, code, version)
}

func (r *RepositoryResolver) FindMBTIModelByQuestionnaire(ctx context.Context, code, version string) (*rulesetmbti.ModelSnapshot, error) {
	if r == nil || r.mbtiCatalog == nil {
		return nil, fmt.Errorf("mbti model catalog is not configured")
	}
	return r.mbtiCatalog.FindMBTIModelByQuestionnaire(ctx, code, version)
}

type ModelInputProvider interface {
	Kind() port.EvaluationModelKind
	ResolveInput(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error)
}

type ModelInputProviderRegistry struct {
	items map[port.EvaluationModelKind]ModelInputProvider
}

func NewModelInputProviderRegistry(providers ...ModelInputProvider) (*ModelInputProviderRegistry, error) {
	registry := &ModelInputProviderRegistry{items: make(map[port.EvaluationModelKind]ModelInputProvider)}
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
	kind := provider.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation input provider kind is empty")
	}
	if _, exists := r.items[kind]; exists {
		return fmt.Errorf("evaluation input provider already registered for kind %s", kind)
	}
	r.items[kind] = provider
	return nil
}

func (r *ModelInputProviderRegistry) Resolve(kind port.EvaluationModelKind) (ModelInputProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation input provider registry is not configured")
	}
	provider, ok := r.items[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", kind)
	}
	return provider, nil
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

func (ScaleModelInputProvider) Kind() port.EvaluationModelKind {
	return port.EvaluationModelKindScale
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
