package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type assessmentGetter struct {
	service *submissionService
}

func (g assessmentGetter) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error) {
	s := g.service
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("获取我的测评详情",
		"action", "get_my_assessment",
		"testee_id", testeeID,
		"assessment_id", assessmentID,
	)

	id := meta.FromUint64(assessmentID)
	a, err := s.repo.FindByID(ctx, id)
	if err != nil {
		l.Errorw("查询测评失败",
			"assessment_id", assessmentID,
			"action", "get_my_assessment",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	if a.TesteeID().Uint64() != testeeID {
		l.Warnw("无权访问测评",
			"action", "get_my_assessment",
			"testee_id", testeeID,
			"assessment_testee_id", a.TesteeID().Uint64(),
			"result", "permission_denied",
		)
		return nil, errors.WithCode(errorCode.ErrForbidden, "无权访问此测评")
	}

	l.Debugw("获取我的测评成功",
		"assessment_id", assessmentID,
		"status", a.Status().String(),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return toAssessmentResult(a)
}

func (g assessmentGetter) GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentResult, error) {
	s := g.service
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("通过答卷ID获取测评详情",
		"action", "get_assessment_by_answersheet",
		"answer_sheet_id", answerSheetID,
	)

	answerSheetRef := assessment.NewAnswerSheetRef(meta.FromUint64(answerSheetID))
	a, err := s.repo.FindByAnswerSheetID(ctx, answerSheetRef)
	if err != nil {
		l.Errorw("通过答卷ID查询测评失败",
			"answer_sheet_id", answerSheetID,
			"action", "get_assessment_by_answersheet",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	l.Debugw("通过答卷ID获取测评成功",
		"answer_sheet_id", answerSheetID,
		"assessment_id", a.ID().Uint64(),
		"status", a.Status().String(),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return toAssessmentResult(a)
}
