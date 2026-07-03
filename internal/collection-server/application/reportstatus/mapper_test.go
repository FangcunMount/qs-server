package reportstatus

import (
	"testing"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

func TestPersonalityTerminalStatusContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status   string
		terminal bool
	}{
		{status: "interpreted", terminal: true},
		{status: "failed", terminal: true},
		{status: "processing", terminal: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := IsTerminalStatus(tc.status); got != tc.terminal {
				t.Fatalf("terminal = %v, want %v", got, tc.terminal)
			}
		})
	}
}

func TestReportStatusHTTPAndWSMappingContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		internal string
		public   string
		terminal bool
	}{
		{name: "completed maps to interpreted", internal: "completed", public: "interpreted", terminal: true},
		{name: "interpreted stays terminal", internal: "interpreted", public: "interpreted", terminal: true},
		{name: "processing stays in flight", internal: "processing", public: "processing", terminal: false},
		{name: "failed terminal", internal: "failed", public: "failed", terminal: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			internal := &evaluationapp.AssessmentStatusResponse{
				Status:          tc.internal,
				Stage:           "stage",
				Message:         "msg",
				NextPollAfterMs: 1500,
			}
			httpStatus := ToPublicAssessmentStatus(internal)
			if httpStatus.Status != tc.public {
				t.Fatalf("http status = %q, want %q", httpStatus.Status, tc.public)
			}

			wsStatus := MedicalView(httpStatus)
			if wsStatus == nil {
				t.Fatal("ws status is nil")
			}
			if wsStatus.Status != tc.public {
				t.Fatalf("ws status = %q, want %q", wsStatus.Status, tc.public)
			}
			if got := IsTerminalStatus(wsStatus.Status); got != tc.terminal {
				t.Fatalf("terminal = %v, want %v", got, tc.terminal)
			}
		})
	}
}

func TestMedicalViewMapsCompletedAndCopiesPayload(t *testing.T) {
	t.Parallel()

	total := 90.5
	risk := "high"
	got := MedicalView(&evaluationapp.AssessmentStatusResponse{
		Status:          "completed",
		Stage:           "scored",
		Message:         "done",
		Reason:          "ok",
		NextPollAfterMs: 100,
		TotalScore:      &total,
		RiskLevel:       &risk,
		UpdatedAt:       123,
	})
	if got == nil {
		t.Fatal("MedicalView() = nil")
	}
	if got.Status != "interpreted" ||
		got.Stage != "scored" ||
		got.Message != "done" ||
		got.Reason != "ok" ||
		got.NextPollAfterMs != 100 ||
		got.UpdatedAt != 123 {
		t.Fatalf("unexpected view: %+v", got)
	}
	if got.TotalScore == nil || *got.TotalScore != total {
		t.Fatalf("total score = %v, want %v", got.TotalScore, total)
	}
	if got.RiskLevel == nil || *got.RiskLevel != risk {
		t.Fatalf("risk level = %v, want %v", got.RiskLevel, risk)
	}
}

func TestPersonalityViewCopiesStatusFields(t *testing.T) {
	t.Parallel()

	got := PersonalityView(StatusFields{
		Status:          "interpreted",
		Stage:           "interpreted",
		Message:         "ready",
		Reason:          "done",
		NextPollAfterMs: 0,
		UpdatedAt:       456,
	})
	if got == nil {
		t.Fatal("PersonalityView() = nil")
	}
	if got.Status != "interpreted" ||
		got.Stage != "interpreted" ||
		got.Message != "ready" ||
		got.Reason != "done" ||
		got.NextPollAfterMs != 0 ||
		got.UpdatedAt != 456 {
		t.Fatalf("unexpected view: %+v", got)
	}
}
