package operator

import (
	"context"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type authorizer struct {
	assessments domainassessment.Repository
	access      AccessChecker
}

func (a authorizer) validateActor(actor Actor) error {
	if actor.OrgID <= 0 || actor.OperatorUserID <= 0 {
		return evalerrors.InvalidArgument("操作者范围不能为空")
	}
	return nil
}

func (a authorizer) loadAssessment(ctx context.Context, actor Actor, id uint64) (*domainassessment.Assessment, error) {
	if err := a.validateActor(actor); err != nil {
		return nil, err
	}
	if a.assessments == nil {
		return nil, evalerrors.ModuleNotConfigured("assessment repository is not configured")
	}
	assessment, err := a.assessments.FindByID(ctx, meta.FromUint64(id))
	if err != nil {
		return nil, evalerrors.AssessmentNotFound(err, "测评不存在")
	}
	if assessment.OrgID() != actor.OrgID {
		return nil, evalerrors.PermissionDenied("测评不属于当前机构")
	}
	if a.access == nil {
		return nil, evalerrors.ModuleNotConfigured("testee access checker is not configured")
	}
	if err := a.access.ValidateTesteeAccess(ctx, actor.OrgID, actor.OperatorUserID, assessment.TesteeID().Uint64()); err != nil {
		return nil, err
	}
	return assessment, nil
}
