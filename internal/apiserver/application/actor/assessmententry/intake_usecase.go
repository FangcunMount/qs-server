package assessmententry

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainRelation "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
)

type historicalIntakeStagePayload struct {
	EntryID              string     `json:"entry_id"`
	TesteeID             string     `json:"testee_id"`
	CreatorRelationID    string     `json:"creator_relation_id"`
	AssignmentRelationID string     `json:"assignment_relation_id"`
	IntakeLogID          uint64     `json:"intake_log_id"`
	ProfileID            *uint64    `json:"profile_id,omitempty"`
	Name                 string     `json:"name"`
	Gender               int8       `json:"gender"`
	Birthday             *time.Time `json:"birthday,omitempty"`
	TesteeCreated        bool       `json:"testee_created"`
	CreatorCreated       bool       `json:"creator_created"`
	AssignmentCreated    bool       `json:"assignment_created"`
}

type intakeUseCase struct {
	service *service
}

func newIntakeUseCase(service *service) *intakeUseCase {
	return &intakeUseCase{service: service}
}

func (u *intakeUseCase) Execute(ctx context.Context, token string, dto IntakeByAssessmentEntryDTO) (*AssessmentEntryIntakeResult, error) {
	state := &intakeState{}
	intakeAt := time.Time{}
	if historical, ok := historicalseed.FromContext(ctx); ok {
		var err error
		intakeAt, err = historicalseed.OccurredAt(ctx, historical.OrgID, historicalseed.StageEntryIntake, time.Now())
		if err != nil {
			return nil, errors.WithCode(code.ErrInvalidArgument, "%v", err)
		}
	}
	attemptCtx, handle, err := stageport.BeginStageAttempt(ctx, u.service.stageRecorder, stageport.Attempt{
		Stage: stageport.StageEntryIntake, BusinessAt: intakeAt, ResourceType: "testee",
	})
	if err != nil {
		return nil, err
	}
	ctx = attemptCtx

	err = u.service.uow.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.resolveEntry(txCtx, token, state); err != nil {
			return err
		}
		if replayed, err := u.restoreHistoricalIntake(txCtx, dto, state); err != nil {
			return err
		} else if replayed {
			return nil
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
		if err := u.service.logIntakeSuccess(txCtx, state); err != nil {
			return err
		}
		if u.service.stageRecorder != nil {
			_, err := stageport.CompleteStage(txCtx, u.service.stageRecorder, historicalIntakeCompletion(dto, state))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resourceID := ""
		if state.testee != nil {
			resourceID = state.testee.ID().String()
		}
		_ = stageport.FailStageAttempt(ctx, u.service.stageRecorder, handle, stageport.Failure{
			Stage: stageport.StageEntryIntake, BusinessAt: intakeAt, ResourceType: "testee", ResourceID: resourceID, Err: err,
		})
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

func historicalIntakeCompletion(dto IntakeByAssessmentEntryDTO, state *intakeState) stageport.Completion {
	payload := historicalIntakeStagePayload{
		EntryID: state.entry.ID().String(), TesteeID: state.testee.ID().String(), CreatorRelationID: state.relation.ID().String(),
		IntakeLogID: state.intakeLogID,
		ProfileID:   dto.ProfileID, Name: dto.Name, Gender: dto.Gender, Birthday: dto.Birthday,
		TesteeCreated: state.testeeCreated, CreatorCreated: state.creatorCreated, AssignmentCreated: state.assignmentCreated,
	}
	if state.assignment != nil {
		payload.AssignmentRelationID = strconv.FormatUint(state.assignment.ID, 10)
	}
	return stageport.Completion{Stage: stageport.StageEntryIntake, BusinessAt: state.intakeAt, ResourceType: "testee", ResourceID: state.testee.ID().String(), Payload: payload}
}

func (u *intakeUseCase) restoreHistoricalIntake(ctx context.Context, dto IntakeByAssessmentEntryDTO, state *intakeState) (bool, error) {
	reader, ok := u.service.stageRecorder.(stageport.CurrentReader)
	if !ok {
		return false, nil
	}
	record, err := reader.FindCurrent(ctx, stageport.StageEntryIntake)
	if err != nil || record == nil {
		return false, err
	}
	var payload historicalIntakeStagePayload
	if err := json.Unmarshal(record.PayloadJSON, &payload); err != nil {
		return false, fmt.Errorf("decode historical intake stage: %w", err)
	}
	testeeID, err := strconv.ParseUint(payload.TesteeID, 10, 64)
	if err != nil || testeeID == 0 {
		return false, fmt.Errorf("historical intake stage has invalid testee_id %q", payload.TesteeID)
	}
	creatorID, err := strconv.ParseUint(payload.CreatorRelationID, 10, 64)
	if err != nil || creatorID == 0 {
		return false, fmt.Errorf("historical intake stage has invalid creator_relation_id %q", payload.CreatorRelationID)
	}
	assignmentID, err := strconv.ParseUint(payload.AssignmentRelationID, 10, 64)
	if err != nil || assignmentID == 0 {
		return false, fmt.Errorf("historical intake stage has invalid assignment_relation_id %q", payload.AssignmentRelationID)
	}
	state.testee, err = u.service.testeeRepo.FindByID(ctx, domainTestee.NewID(testeeID))
	if err != nil || state.testee == nil {
		return false, fmt.Errorf("restore historical intake testee %d: %w", testeeID, err)
	}
	state.relation, err = u.service.relationRepo.FindByID(ctx, domainRelation.NewID(creatorID))
	if err != nil || state.relation == nil {
		return false, fmt.Errorf("restore historical creator relation %d: %w", creatorID, err)
	}
	assignment, err := u.service.relationRepo.FindByID(ctx, domainRelation.NewID(assignmentID))
	if err != nil || assignment == nil {
		return false, fmt.Errorf("restore historical assignment relation %d: %w", assignmentID, err)
	}
	state.assignment = toRelationSummaryResult(assignment)
	state.testeeCreated = payload.TesteeCreated
	state.creatorCreated = payload.CreatorCreated
	state.assignmentCreated = payload.AssignmentCreated
	state.intakeLogID = payload.IntakeLogID
	_, err = stageport.CompleteStage(ctx, u.service.stageRecorder, historicalIntakeCompletion(dto, state))
	return err == nil, err
}

func (u *intakeUseCase) resolveEntry(ctx context.Context, token string, state *intakeState) error {
	entry, clinician, err := u.service.resolveEntry(ctx, token)
	if err != nil {
		return err
	}
	state.entry = entry
	state.clinician = clinician
	intakeAt, err := historicalseed.OccurredAt(ctx, uint64(entry.OrgID()), historicalseed.StageEntryIntake, time.Now())
	if err != nil {
		return errors.WithCode(code.ErrInvalidArgument, "%v", err)
	}
	state.intakeAt = intakeAt
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
	relation, created, err := u.service.ensureCreatorRelation(ctx, state.entry, state.testee, state.intakeAt)
	if err != nil {
		return err
	}
	state.relation = relation
	state.creatorCreated = created
	return nil
}

func (u *intakeUseCase) ensureAccessAssignment(ctx context.Context, state *intakeState) error {
	assignedRelation, assignmentCreated, err := u.service.ensureAssignmentRelation(ctx, state.entry, state.testee, state.intakeAt)
	if err != nil {
		return err
	}
	state.assignmentCreated = assignmentCreated
	state.assignment = toRelationSummaryResult(assignedRelation)
	return nil
}
