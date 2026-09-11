package aibridge

import (
	"context"
	"errors"
	"testing"
	"time"

	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/google/uuid"
)

type accessStub struct{ err error }

func (a accessStub) AuthorizeOwnAssessment(context.Context, uint64, uint64) error { return a.err }

type sourceStub struct {
	current *source.Current
	calls   int
}

func (s *sourceStub) ResolveCurrent(context.Context, meta.ID) (*source.Current, error) {
	s.calls++
	return s.current, nil
}

type stagingStore struct {
	Store
	requests []Start
}

func (s *stagingStore) StageStart(_ context.Context, r Start) error {
	s.requests = append(s.requests, r)
	return nil
}
func TestParticipantSnapshotAuthorizationAndSourceBinding(t *testing.T) {
	r, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(99), GenerationID: meta.FromUint64(100), OutcomeID: meta.FromUint64(101), InterpretationRunID: meta.FromUint64(102),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7}, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionCurrent,
		ContentSchemaVersion: "standard-v1", BuilderIdentity: "test", GeneratedAt: time.Now(), Content: report.Content{Conclusion: "真实标准结论"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name           string
		denied         bool
		org            string
		testee, report uint64
		wantErr        bool
	}{
		{"accepted", false, "1", 7, 99, false}, {"revoked", true, "1", 7, 99, true}, {"wrong org", false, "2", 7, 99, true}, {"wrong testee", false, "1", 8, 99, true}, {"stale report", false, "1", 7, 98, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var denied error
			if tc.denied {
				denied = errors.New("denied")
			}
			sources := &sourceStub{current: &source.Current{Report: r, Outcome: evaluationfact.NewRecord(evaluationfact.NewRecordInput{
				ID: r.OutcomeID(), OrgID: 1, TesteeID: 7, AssessmentID: meta.FromUint64(42),
			})}}
			store := &stagingStore{}
			p := Participant{Access: accessStub{denied}, Sources: sources, Bridge: &Service{Store: store}}
			requestID := uuid.NewString()
			err := p.Request(context.Background(), Actor{tc.org, "parent"}, tc.testee, 42, tc.report, requestID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if tc.wantErr && len(store.requests) != 0 {
				t.Fatal("unauthorized or stale input was staged")
			}
			if tc.denied && sources.calls != 0 {
				t.Fatal("read before authorization")
			}
			if !tc.wantErr {
				sources.current = nil
				if err := p.Request(context.Background(), Actor{tc.org, "parent"}, tc.testee, 42, tc.report, requestID); err != nil {
					t.Fatal(err)
				}
				if sources.calls != 1 || len(store.requests) != 1 {
					t.Fatal("retry re-read or re-staged report")
				}
				e := store.requests[0].Evidence[0]
				if e.ReportID != "99" || e.SourceVersion != "standard-v1:101" || len(e.Facts) != 1 {
					t.Fatalf("snapshot=%+v", e)
				}
			}
		})
	}
}

func (s *stagingStore) Original(_ context.Context, id string) (*Start, error) {
	for _, r := range s.requests {
		if r.RequestID == id {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

type readingStore struct {
	Store
	request *Start
	event   *Event
	reads   int
}

func (s *readingStore) Original(context.Context, string) (*Start, error) {
	s.reads++
	return s.request, nil
}
func (s *readingStore) Projection(context.Context, string) (*Event, error) {
	s.reads++
	return s.event, nil
}
func TestParticipantWorkflowReadRequiresCurrentAccessAndRequestOwnership(t *testing.T) {
	for _, scenario := range []string{"pending", "state", "revoked", "org", "owner", "testee", "assessment", "corrupt_projection"} {
		t.Run(scenario, func(t *testing.T) {
			id := uuid.NewString()
			actor := Actor{"1", "parent"}
			request := &Start{RequestID: id, Actor: actor, TesteeID: "7", AssessmentIDs: []string{"42"}}
			event := &Event{RequestID: id, Actor: actor, TesteeID: "7", Status: "running"}
			store := &readingStore{request: request, event: event}
			var denied error
			switch scenario {
			case "pending":
				store.event = nil
			case "revoked":
				denied = errors.New("revoked")
			case "org":
				request.Actor.OrgID = "2"
			case "owner":
				request.Actor.SubjectID = "other"
			case "testee":
				request.TesteeID = "8"
			case "assessment":
				request.AssessmentIDs = []string{"43"}
			case "corrupt_projection":
				event.Actor.SubjectID = "other"
			}
			p := Participant{Access: accessStub{denied}, Bridge: &Service{Store: store}}
			result, err := p.Read(context.Background(), actor, 7, 42, id)
			if scenario == "pending" || scenario == "state" {
				if err != nil || result != store.event {
					t.Fatalf("read=%v error=%v", result, err)
				}
			} else if err == nil || result != nil {
				t.Fatal("unauthorized projection returned")
			}
			if scenario == "revoked" && store.reads != 0 {
				t.Fatal("revoked request read persistence")
			}
		})
	}
}
