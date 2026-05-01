package assessmententry

import (
	"context"
	"time"
)

type intakeUseCase struct {
	service *service
}

func newIntakeUseCase(service *service) *intakeUseCase {
	return &intakeUseCase{service: service}
}

func (u *intakeUseCase) Execute(ctx context.Context, token string, dto IntakeByAssessmentEntryDTO) (*AssessmentEntryIntakeResult, error) {
	state := &intakeState{intakeAt: time.Now()}

	err := u.service.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.resolveEntry(txCtx, token, state); err != nil {
			return err
		}
		if err := u.validateProfile(txCtx, dto); err != nil {
			return err
		}
		if err := u.resolveOrCreateTestee(txCtx, dto, state); err != nil {
			return err
		}
		if err := u.ensureCreatorRelation(txCtx, state); err != nil {
			return err
		}
		if err := u.ensureAccessAssignment(txCtx, state); err != nil {
			return err
		}
		return u.service.stageIntakeBehaviorEvents(txCtx, state)
	})
	if err != nil {
		return nil, err
	}

	return &AssessmentEntryIntakeResult{
		Entry:      toAssessmentEntryResult(state.entry),
		Clinician:  toClinicianSummaryResult(state.clinician),
		Testee:     toTesteeSummaryResult(state.testee),
		Relation:   toRelationSummaryResult(state.relation),
		Assignment: state.assignment,
	}, nil
}

func (u *intakeUseCase) resolveEntry(ctx context.Context, token string, state *intakeState) error {
	entry, clinician, err := u.service.resolveEntry(ctx, token)
	if err != nil {
		return err
	}
	state.entry = entry
	state.clinician = clinician
	return nil
}

func (u *intakeUseCase) validateProfile(ctx context.Context, dto IntakeByAssessmentEntryDTO) error {
	return u.service.validateIntakeProfile(ctx, dto)
}

func (u *intakeUseCase) resolveOrCreateTestee(ctx context.Context, dto IntakeByAssessmentEntryDTO, state *intakeState) error {
	testeeItem, created, err := u.service.resolveIntakeTestee(ctx, state.entry, dto)
	if err != nil {
		return err
	}
	state.testee = testeeItem
	state.testeeCreated = created
	return nil
}

func (u *intakeUseCase) ensureCreatorRelation(ctx context.Context, state *intakeState) error {
	relation, err := u.service.ensureCreatorRelation(ctx, state.entry, state.testee)
	if err != nil {
		return err
	}
	state.relation = relation
	return nil
}

func (u *intakeUseCase) ensureAccessAssignment(ctx context.Context, state *intakeState) error {
	assignedRelation, assignmentCreated, err := u.service.ensureAssignmentRelation(ctx, state.entry, state.testee)
	if err != nil {
		return err
	}
	state.assignmentCreated = assignmentCreated
	state.assignment = toRelationSummaryResult(assignedRelation)
	return nil
}
