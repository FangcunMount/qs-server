package systemgovernance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type currentReminderRecoveryStub struct{ inspections, writes, resumes, reads int }

func (s *currentReminderRecoveryStub) InspectReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ReminderGapInspection, error) {
	s.inspections++
	return &ReminderGapInspection{ReadOnly: true}, nil
}
func (s *currentReminderRecoveryStub) AuthorizeReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, error) {
	s.writes++
	return &ActionRunResult{Status: "succeeded"}, nil
}
func (s *currentReminderRecoveryStub) ResumeReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, error) {
	s.resumes++
	return &ActionRunResult{Status: "pending_reconciliation"}, nil
}
func (s *currentReminderRecoveryStub) ResolveReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, bool, error) {
	s.reads++
	return &ActionRunResult{Status: "pending_reconciliation"}, true, nil
}

func currentReminderRequest() ReminderGapRecoveryRequest {
	return ReminderGapRecoveryRequest{RequestID: "original-operation", TaskID: "100", OpeningEventID: "original-opening", OpenedBefore: time.Now().Add(-time.Minute), EvidenceReference: "reviewed-original", Reason: "original gap", Confirm: true, SourceProtocol: TransactionReminderSource, ExpectedScheduleRevision: 1, SourceFingerprint: strings.Repeat("ab", 32)}
}

func TestCurrentReminderRecoveryReadsSurviveDisabledWrites(t *testing.T) {
	backend := &currentReminderRecoveryStub{}
	facade := NewFacade(FacadeDeps{CurrentReminderGapRecovery: backend}).(CurrentReminderGapRecovery)
	req := currentReminderRequest()
	_, err := facade.InspectReminderGap(t.Context(), 7, 11, req)
	require.NoError(t, err)
	_, _, err = facade.ResolveReminderGap(t.Context(), 7, 11, req)
	require.NoError(t, err)
	_, err = facade.AuthorizeReminderGap(t.Context(), 7, 11, req)
	require.Error(t, err)
	_, err = facade.ResumeReminderGap(t.Context(), 7, 11, req)
	require.Error(t, err)
	require.Equal(t, 1, backend.inspections)
	require.Equal(t, 1, backend.reads)
	require.Zero(t, backend.writes)
	require.Zero(t, backend.resumes)
}

func TestCurrentReminderProtocolDoesNotReuseOutboxVersion(t *testing.T) {
	req := currentReminderRequest()
	require.NoError(t, ValidateReminderGapRecovery(7, 11, req))
	for _, name := range []string{"version", "revision", "fingerprint", "confirm", "protocol"} {
		t.Run(name, func(t *testing.T) {
			changed := req
			switch name {
			case "version":
				changed.ExpectedVersion = 1
			case "revision":
				changed.ExpectedScheduleRevision = 0
			case "fingerprint":
				changed.SourceFingerprint = ""
			case "confirm":
				changed.Confirm = false
			case "protocol":
				changed.SourceProtocol = "other"
			}
			require.Error(t, ValidateReminderGapRecovery(7, 11, changed))
		})
	}
	inspection := req
	inspection.Confirm = false
	inspection.SourceFingerprint = ""
	inspection.RequestID = ""
	inspection.OpeningEventID = ""
	require.NoError(t, ValidateTransactionReminderGap(7, 11, inspection, true))
	legacy := ReminderGapRecoveryRequest{RequestID: "legacy", TaskID: "100", OpeningEventID: "opening", ExpectedVersion: 1, OpenedBefore: req.OpenedBefore, EvidenceReference: "review", Reason: "reason", Confirm: true}
	require.NoError(t, ValidateReminderGapRecovery(7, 11, legacy))
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "source_protocol")
	require.NotContains(t, string(raw), "source_fingerprint")
	require.NotContains(t, string(raw), "expected_schedule_revision")
}
