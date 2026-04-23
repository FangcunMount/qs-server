package assessmententry

import (
	"context"
	"testing"
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
