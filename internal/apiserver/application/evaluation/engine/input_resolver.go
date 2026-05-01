package engine

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type EvaluationInput struct {
	MedicalScale  *scale.MedicalScale
	AnswerSheet   *answersheet.AnswerSheet
	Questionnaire *questionnaire.Questionnaire
}

type EvaluationInputResolver interface {
	Resolve(ctx context.Context, assessment *assessment.Assessment) (*EvaluationInput, error)
}

type inputResolveError struct {
	apiErr        error
	failureReason string
}

func (e *inputResolveError) Error() string {
	return e.apiErr.Error()
}

func (e *inputResolveError) Unwrap() error {
	return e.apiErr
}

func (e *inputResolveError) FailureReason() string {
	return e.failureReason
}

type repositoryInputResolver struct {
	scaleRepo         scale.Repository
	answerSheetRepo   answersheet.Repository
	questionnaireRepo questionnaire.Repository
}

func NewRepositoryInputResolver(
	scaleRepo scale.Repository,
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
) EvaluationInputResolver {
	return &repositoryInputResolver{
		scaleRepo:         scaleRepo,
		answerSheetRepo:   answerSheetRepo,
		questionnaireRepo: questionnaireRepo,
	}
}

func (r *repositoryInputResolver) Resolve(ctx context.Context, a *assessment.Assessment) (*EvaluationInput, error) {
	if a == nil {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "测评不能为空")
	}

	medicalScale, err := r.loadMedicalScale(ctx, a)
	if err != nil {
		return nil, err
	}
	answerSheet, err := r.loadAnswerSheet(ctx, a)
	if err != nil {
		return nil, err
	}
	qnr, err := r.loadQuestionnaire(ctx, answerSheet)
	if err != nil {
		return nil, err
	}

	return &EvaluationInput{
		MedicalScale:  medicalScale,
		AnswerSheet:   answerSheet,
		Questionnaire: qnr,
	}, nil
}

func (r *repositoryInputResolver) loadMedicalScale(ctx context.Context, a *assessment.Assessment) (*scale.MedicalScale, error) {
	l := logger.L(ctx)
	scaleCode := a.MedicalScaleRef().Code().String()
	l.Debugw("加载量表数据",
		"scale_code", scaleCode,
		"action", "read",
		"resource", "scale",
	)

	medicalScale, err := r.scaleRepo.FindByCode(ctx, scaleCode)
	if err != nil {
		l.Errorw("加载量表失败",
			"scale_code", scaleCode,
			"action", "read",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, newInputResolveError(err, errorCode.ErrMedicalScaleNotFound, "量表不存在", "加载量表失败")
	}

	l.Debugw("量表数据加载成功",
		"scale_code", scaleCode,
		"scale_title", medicalScale.GetTitle(),
		"result", "success",
	)
	return medicalScale, nil
}

func (r *repositoryInputResolver) loadAnswerSheet(ctx context.Context, a *assessment.Assessment) (*answersheet.AnswerSheet, error) {
	l := logger.L(ctx)
	answerSheetID := a.AnswerSheetRef().ID()
	l.Debugw("加载答卷数据",
		"answer_sheet_id", answerSheetID,
		"action", "read",
		"resource", "answersheet",
	)

	answerSheet, err := r.answerSheetRepo.FindByID(ctx, answerSheetID)
	if err != nil {
		l.Errorw("加载答卷失败",
			"answer_sheet_id", answerSheetID,
			"action", "evaluate_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, newInputResolveError(err, errorCode.ErrAnswerSheetNotFound, "答卷不存在", "加载答卷失败")
	}

	l.Debugw("答卷数据加载成功",
		"answer_sheet_id", answerSheetID,
		"questionnaire_code", func() string { code, _, _ := answerSheet.QuestionnaireInfo(); return code }(),
		"result", "success",
	)
	return answerSheet, nil
}

func (r *repositoryInputResolver) loadQuestionnaire(ctx context.Context, answerSheet *answersheet.AnswerSheet) (*questionnaire.Questionnaire, error) {
	l := logger.L(ctx)
	qCode, qVersion, _ := answerSheet.QuestionnaireInfo()
	l.Debugw("加载问卷数据",
		"questionnaire_code", qCode,
		"questionnaire_version", qVersion,
		"action", "read",
		"resource", "questionnaire",
	)

	qnr, err := r.questionnaireRepo.FindByCodeVersion(ctx, qCode, qVersion)
	if err != nil {
		l.Errorw("加载问卷失败，评估终止",
			"questionnaire_code", qCode,
			"questionnaire_version", qVersion,
			"error", err.Error(),
		)
		return nil, newInputResolveError(err, errorCode.ErrQuestionnaireNotFound, "加载问卷失败", "加载问卷失败")
	}
	if qnr == nil {
		err = errors.WithCode(errorCode.ErrQuestionnaireNotFound, "问卷不存在或版本不匹配")
		l.Errorw("加载问卷失败，未命中答卷要求的精确版本",
			"questionnaire_code", qCode,
			"questionnaire_version", qVersion,
			"error", err.Error(),
		)
		return nil, &inputResolveError{apiErr: err, failureReason: "加载问卷失败: " + err.Error()}
	}

	l.Debugw("问卷数据加载成功",
		"questionnaire_code", qCode,
		"questionnaire_version", qVersion,
		"question_count", len(qnr.GetQuestions()),
		"result", "success",
	)
	return qnr, nil
}

func newInputResolveError(err error, code int, message, failurePrefix string) error {
	return &inputResolveError{
		apiErr:        errors.WrapC(err, code, "%s", message),
		failureReason: fmt.Sprintf("%s: %s", failurePrefix, err.Error()),
	}
}

func inputResolveFailureReason(err error) string {
	var resolveErr *inputResolveError
	if stderrors.As(err, &resolveErr) {
		return resolveErr.FailureReason()
	}
	return "评估输入加载失败: " + err.Error()
}
