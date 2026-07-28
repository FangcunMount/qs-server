package assessmententry

import (
	"context"
	"testing"
	"time"

	domainAssessmentEntry "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	domainClinician "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

type profileReaderStub struct {
	enabled       bool
	lastProfileID string
	err           error
}

func (s *profileReaderStub) IsEnabled() bool {
	return s.enabled
}

func (s *profileReaderStub) ValidateProfileExists(_ context.Context, profileID string) error {
	s.lastProfileID = profileID
	return s.err
}

func TestServiceValidateIntakeProfileUsesProfileReader(t *testing.T) {
	profileID := uint64(88)
	reader := &profileReaderStub{enabled: true}
	svc := &service{profileReader: reader}

	err := svc.validateIntakeProfile(context.Background(), IntakeByAssessmentEntryDTO{ProfileID: &profileID})
	if err != nil {
		t.Fatalf("validateIntakeProfile() error = %v", err)
	}
	if reader.lastProfileID != "88" {
		t.Fatalf("profileID = %q, want 88", reader.lastProfileID)
	}
}

type recordingIntakeLogWriter struct {
	logID             uint64
	orgID             int64
	clinicianID       uint64
	entryID           uint64
	testeeID          uint64
	intakeAt          time.Time
	testeeCreated     bool
	assignmentCreated bool
}

func (w *recordingIntakeLogWriter) LogIntake(_ context.Context, orgID int64, clinicianID, entryID, testeeID uint64, intakeAt time.Time, testeeCreated, assignmentCreated bool) (uint64, error) {
	w.orgID = orgID
	w.clinicianID = clinicianID
	w.entryID = entryID
	w.testeeID = testeeID
	w.intakeAt = intakeAt
	w.testeeCreated = testeeCreated
	w.assignmentCreated = assignmentCreated
	if w.logID == 0 {
		w.logID = 401
	}
	return w.logID, nil
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

	state := &intakeState{
		entry:             entry,
		testee:            testee,
		intakeAt:          intakeAt,
		testeeCreated:     true,
		assignmentCreated: true,
	}
	err := svc.logIntakeSuccess(context.Background(), state)
	if err != nil {
		t.Fatalf("logIntakeSuccess() error = %v", err)
	}
	if writer.orgID != 9 || writer.clinicianID != 101 || writer.entryID != 201 || writer.testeeID != 301 {
		t.Fatalf("logged identity = org:%d clinician:%d entry:%d testee:%d", writer.orgID, writer.clinicianID, writer.entryID, writer.testeeID)
	}
	if !writer.intakeAt.Equal(intakeAt) || !writer.testeeCreated || !writer.assignmentCreated {
		t.Fatalf("logged funnel flags/time = time:%v testee:%v assignment:%v", writer.intakeAt, writer.testeeCreated, writer.assignmentCreated)
	}
	if state.intakeLogID != 401 {
		t.Fatalf("intake log id=%d, want 401", state.intakeLogID)
	}
}

type recordingResolveLogWriter struct {
	logID       uint64
	orgID       int64
	clinicianID uint64
	entryID     uint64
	resolvedAt  time.Time
}

func (w *recordingResolveLogWriter) LogResolve(_ context.Context, orgID int64, clinicianID, entryID uint64, resolvedAt time.Time) (uint64, error) {
	w.orgID = orgID
	w.clinicianID = clinicianID
	w.entryID = entryID
	w.resolvedAt = resolvedAt
	if w.logID == 0 {
		w.logID = 501
	}
	return w.logID, nil
}

func TestServiceLogResolveSuccessPersistsEntryOpenFact(t *testing.T) {
	t.Parallel()

	resolvedAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
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
	writer := &recordingResolveLogWriter{}
	svc := &service{resolveLog: writer}

	logID, err := svc.logResolveSuccess(context.Background(), entry, resolvedAt)
	if err != nil {
		t.Fatalf("logResolveSuccess() error = %v", err)
	}
	if writer.orgID != 9 || writer.clinicianID != 101 || writer.entryID != 201 {
		t.Fatalf("logged identity = org:%d clinician:%d entry:%d", writer.orgID, writer.clinicianID, writer.entryID)
	}
	if !writer.resolvedAt.Equal(resolvedAt) {
		t.Fatalf("resolvedAt = %v, want %v", writer.resolvedAt, resolvedAt)
	}
	if logID != 501 {
		t.Fatalf("resolve log id=%d, want 501", logID)
	}
}
