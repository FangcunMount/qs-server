package assessmententry

import (
	"context"
	"testing"
	"time"

	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

type guardianshipReaderStub struct {
	enabled     bool
	lastChildID string
	err         error
}

func (s *guardianshipReaderStub) IsEnabled() bool {
	return s.enabled
}

func (s *guardianshipReaderStub) ValidateChildExists(_ context.Context, childID string) error {
	s.lastChildID = childID
	return s.err
}

func TestServiceValidateIntakeProfileUsesGuardianshipReader(t *testing.T) {
	profileID := uint64(88)
	reader := &guardianshipReaderStub{enabled: true}
	svc := &service{guardianshipSvc: reader}

	err := svc.validateIntakeProfile(context.Background(), IntakeByAssessmentEntryDTO{ProfileID: &profileID})
	if err != nil {
		t.Fatalf("validateIntakeProfile() error = %v", err)
	}
	if reader.lastChildID != "88" {
		t.Fatalf("childID = %q, want 88", reader.lastChildID)
	}
}

type recordingIntakeLogWriter struct {
	orgID             int64
	clinicianID       uint64
	entryID           uint64
	testeeID          uint64
	intakeAt          time.Time
	testeeCreated     bool
	assignmentCreated bool
}

func (w *recordingIntakeLogWriter) LogIntake(_ context.Context, orgID int64, clinicianID, entryID, testeeID uint64, intakeAt time.Time, testeeCreated, assignmentCreated bool) error {
	w.orgID = orgID
	w.clinicianID = clinicianID
	w.entryID = entryID
	w.testeeID = testeeID
	w.intakeAt = intakeAt
	w.testeeCreated = testeeCreated
	w.assignmentCreated = assignmentCreated
	return nil
}

func TestServiceLogIntakeSuccessPersistsFunnelFacts(t *testing.T) {
	t.Parallel()

	intakeAt := time.Date(2026, 5, 6, 11, 0, 0, 0, time.UTC)
	entry := domainAssessmentEntry.NewAssessmentEntry(
		9,
		domainClinician.NewID(101),
		"entry-token",
		domainAssessmentEntry.TargetTypeScale,
		"scale-code",
		"v1",
		true,
		nil,
	)
	entry.SetID(domainAssessmentEntry.NewID(201))
	testee := domainTestee.NewTestee(9, "testee", domainTestee.GenderUnknown, nil)
	testee.SetID(domainTestee.NewID(301))
	writer := &recordingIntakeLogWriter{}
	svc := &service{intakeLog: writer}

	err := svc.logIntakeSuccess(context.Background(), &intakeState{
		entry:             entry,
		testee:            testee,
		intakeAt:          intakeAt,
		testeeCreated:     true,
		assignmentCreated: true,
	})
	if err != nil {
		t.Fatalf("logIntakeSuccess() error = %v", err)
	}
	if writer.orgID != 9 || writer.clinicianID != 101 || writer.entryID != 201 || writer.testeeID != 301 {
		t.Fatalf("logged identity = org:%d clinician:%d entry:%d testee:%d", writer.orgID, writer.clinicianID, writer.entryID, writer.testeeID)
	}
	if !writer.intakeAt.Equal(intakeAt) || !writer.testeeCreated || !writer.assignmentCreated {
		t.Fatalf("logged funnel flags/time = time:%v testee:%v assignment:%v", writer.intakeAt, writer.testeeCreated, writer.assignmentCreated)
	}
}
