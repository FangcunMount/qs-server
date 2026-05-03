package evaluationinput

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type RepositoryResolver struct {
	scaleCatalog        port.ScaleCatalog
	answerSheetReader   port.AnswerSheetReader
	questionnaireReader port.QuestionnaireReader
}

// NewRepositoryResolver builds the current compatibility adapter from Survey/Scale
// command repositories. New snapshot sources should implement the catalog/read-model
// ports and be wired through NewResolver instead of adding more repository/domain
// dependencies outside repository_resolver.go and snapshot_mappers.go.
func NewRepositoryResolver(
	scaleRepo scale.Repository,
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
) *RepositoryResolver {
	return NewResolver(
		NewRepositoryScaleSnapshotCatalog(scaleRepo),
		NewRepositoryAnswerSheetSnapshotReader(answerSheetRepo),
		NewRepositoryQuestionnaireSnapshotReader(questionnaireRepo),
	)
}

func NewResolver(
	scaleCatalog port.ScaleCatalog,
	answerSheetReader port.AnswerSheetReader,
	questionnaireReader port.QuestionnaireReader,
) *RepositoryResolver {
	return &RepositoryResolver{
		scaleCatalog:        scaleCatalog,
		answerSheetReader:   answerSheetReader,
		questionnaireReader: questionnaireReader,
	}
}

func (r *RepositoryResolver) Resolve(ctx context.Context, ref port.InputRef) (*port.InputSnapshot, error) {
	medicalScale, err := r.scaleCatalog.GetScale(ctx, ref.MedicalScaleCode)
	if err != nil {
		return nil, err
	}
	answerSheet, err := r.answerSheetReader.GetAnswerSheet(ctx, ref.AnswerSheetID)
	if err != nil {
		return nil, err
	}
	qnr, err := r.questionnaireReader.GetQuestionnaire(ctx, answerSheet.QuestionnaireCode, answerSheet.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}

	return &port.InputSnapshot{
		MedicalScale:  medicalScale,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}

func (r *RepositoryResolver) GetScale(ctx context.Context, code string) (*port.ScaleSnapshot, error) {
	return r.scaleCatalog.GetScale(ctx, code)
}

type RepositoryScaleSnapshotCatalog struct {
	repo scale.Repository
}

func NewRepositoryScaleSnapshotCatalog(repo scale.Repository) *RepositoryScaleSnapshotCatalog {
	return &RepositoryScaleSnapshotCatalog{repo: repo}
}

func (r *RepositoryScaleSnapshotCatalog) GetScale(ctx context.Context, code string) (*port.ScaleSnapshot, error) {
	l := logger.L(ctx)
	l.Debugw("加载量表数据",
		"scale_code", code,
		"action", "read",
		"resource", "scale",
	)

	medicalScale, err := r.repo.FindByCode(ctx, code)
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
