package operator

import (
	"context"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"

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

func (a authorizer) loadAssessment(ctx context.Context, actor Actor, id uint64, action string) (*domainassessment.Assessment, error) {
	return a.loadAssessmentResource(ctx, actor, id, appauthz.AssessmentResource, action)
}

func (a authorizer) loadAssessmentResource(ctx context.Context, actor Actor, id uint64, resource, action string) (*domainassessment.Assessment, error) {
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
	if err := a.access.ValidateTesteeStoreAccess(ctx, actor.OrgID, actor.OperatorUserID, assessment.TesteeID().Uint64(), resource, action); err != nil {
		return nil, err
	}
	return assessment, nil
}

// AssessmentPermissionAuthorizer checks ownership using the caller's resource/action,
// without returning an assessment or borrowing assessment-read privileges.
type AssessmentPermissionAuthorizer interface {
	AuthorizeAssessmentResource(context.Context, Actor, uint64, string, string) error
}

func (s *queryService) AuthorizeAssessmentResource(ctx context.Context, actor Actor, id uint64, resource, action string) error {
	_, err := (authorizer{assessments: s.assessments, access: s.access}).loadAssessmentResource(ctx, actor, id, resource, action)
	return err
}
