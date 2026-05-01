package relation

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AssignmentRequest describes a clinician-testee relation assignment command.
type AssignmentRequest struct {
	OrgID        int64
	ClinicianID  clinician.ID
	TesteeID     testee.ID
	RelationType RelationType
	SourceType   SourceType
	SourceID     *uint64
	Now          time.Time
}

// AssignmentPlan is the domain decision for relation assignment.
type AssignmentPlan struct {
	ReuseRelation *ClinicianTesteeRelation
	Unbind        []*ClinicianTesteeRelation
	Create        *ClinicianTesteeRelation
}

// AssignmentPolicy decides how to reuse, replace, or create assignment relations.
type AssignmentPolicy interface {
	PlanAssignment(
		request AssignmentRequest,
		activePrimary *ClinicianTesteeRelation,
		activeAccessRelation *ClinicianTesteeRelation,
	) (*AssignmentPlan, error)
}

type assignmentPolicy struct{}

// NewAssignmentPolicy creates a clinician-testee assignment policy.
func NewAssignmentPolicy() AssignmentPolicy {
	return &assignmentPolicy{}
}

func (p *assignmentPolicy) PlanAssignment(
	request AssignmentRequest,
	activePrimary *ClinicianTesteeRelation,
	activeAccessRelation *ClinicianTesteeRelation,
) (*AssignmentPlan, error) {
	if !IsSupportedAssignmentRelationType(request.RelationType) {
		return nil, errors.WithCode(code.ErrInvalidArgument, "unsupported clinician relation type")
	}

	plan := &AssignmentPlan{Unbind: make([]*ClinicianTesteeRelation, 0, 2)}
	if request.RelationType == RelationTypePrimary && activePrimary != nil {
		if activePrimary.ClinicianID() == request.ClinicianID {
			plan.ReuseRelation = activePrimary
			return plan, nil
		}
		activePrimary.Unbind(request.Now)
		plan.Unbind = append(plan.Unbind, activePrimary)
	}

	if activeAccessRelation != nil {
		if activeAccessRelation.RelationType() == request.RelationType {
			plan.ReuseRelation = activeAccessRelation
			return plan, nil
		}
		activeAccessRelation.Unbind(request.Now)
		plan.Unbind = append(plan.Unbind, activeAccessRelation)
	}

	plan.Create = NewClinicianTesteeRelation(
		request.OrgID,
		request.ClinicianID,
		request.TesteeID,
		request.RelationType,
		request.SourceType,
		request.SourceID,
		true,
		request.Now,
		nil,
	)
	return plan, nil
}
