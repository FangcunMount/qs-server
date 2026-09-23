package aibridge

import (
	"context"
	"errors"
	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"testing"
	"time"
)

type sourceErrorStub struct {
	err   error
	calls int
}

func (s *sourceErrorStub) ResolveCurrent(context.Context, meta.ID) (*source.Current, error) {
	s.calls++
	return nil, s.err
}

func TestWorkflowSourceAuthorizesBeforeReadingAndDistinguishesUnavailableFacts(t *testing.T) {
	denied := errors.New("revoked")
	for _, tc := range []struct {
		name                 string
		accessErr, sourceErr error
		want                 string
		fail                 bool
	}{
		{"revoked", denied, nil, "", true},
		{"no report", nil, source.ErrNotReady, "not_ready", false},
		{"legacy", nil, source.ErrNotApplicable, "not_applicable", false},
		{"inconsistent", nil, source.ErrInconsistent, "", true},
		{"missing current", nil, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &sourceErrorStub{err: tc.sourceErr}
			p := &Participant{Access: accessStub{tc.accessErr}, Sources: reader}
			got, err := p.Source(context.Background(), Actor{"1", "parent"}, 7, 42)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v", err)
			}
			if tc.accessErr != nil && (reader.calls != 0 || !errors.Is(err, denied)) {
				t.Fatal("read without access")
			}
			if !tc.fail && (got.Status != tc.want || got.ReportID != "" || got.SourceVersion != "") {
				t.Fatalf("unexpected provenance: %#v", got)
			}
		})
	}
}

func TestWorkflowSourceReturnsAuthorizedImmutableIdentityWithoutStaging(t *testing.T) {
	r, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(99), GenerationID: meta.FromUint64(100), OutcomeID: meta.FromUint64(101), InterpretationRunID: meta.FromUint64(102),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7}, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionCurrent,
		ContentSchemaVersion: "standard-v1", BuilderIdentity: "test", GeneratedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &sourceStub{current: &source.Current{Report: r, Outcome: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: r.OutcomeID(), OrgID: 1, TesteeID: 7, AssessmentID: meta.FromUint64(42)})}}
	// No Bridge/Store is configured: source discovery cannot stage or invoke a model.
	p := &Participant{Access: accessStub{}, Sources: reader, Eligibility: eligibilityReadFunc(func(context.Context, Actor, string, []string, []EvidenceItem) (*Eligibility, error) {
		return &Eligibility{Status: "available"}, nil
	})}
	got, err := p.Source(context.Background(), Actor{"1", "parent"}, 7, 42)
	if err != nil || got == nil || got.Status != "ready" || got.ReportID != "99" || got.SourceVersion != "standard-v1:101" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	for _, tc := range []struct {
		org                string
		testee, assessment uint64
	}{{"2", 7, 42}, {"1", 8, 42}, {"1", 7, 43}} {
		got, err := p.Source(context.Background(), Actor{tc.org, "parent"}, tc.testee, tc.assessment)
		if got != nil || !errors.Is(err, source.ErrInconsistent) {
			t.Fatal("foreign report source exposed")
		}
	}
	reader.current.Outcome = nil
	if _, err := p.Source(context.Background(), Actor{"1", "parent"}, 7, 42); !errors.Is(err, source.ErrInconsistent) {
		t.Fatal("incomplete provenance accepted")
	}
}

type eligibilityReadFunc func(context.Context, Actor, string, []string, []EvidenceItem) (*Eligibility, error)

func (f eligibilityReadFunc) CheckEligibility(ctx context.Context, actor Actor, testee string, assessments []string, evidence []EvidenceItem) (*Eligibility, error) {
	return f(ctx, actor, testee, assessments, evidence)
}
func TestMBTISourceChecksCapabilityOnlyAfterAuthorizationAndFrozenSource(t *testing.T) {
	current := mbtiSnapshotSource(t, mbtiSnapshotContent(false))
	association := current.Report.Association()
	calls := 0
	response := &Eligibility{Status: "unavailable", ReasonCode: "publication_missing"}
	var dependencyErr error
	checker := eligibilityReadFunc(func(_ context.Context, actor Actor, testee string, assessments []string, evidence []EvidenceItem) (*Eligibility, error) {
		calls++
		if actor.OrgID != "1" || len(assessments) != 1 || len(evidence) != 1 || evidence[0].ReportID != current.Report.ID().String() || len(evidence[0].Facts) != 1 || evidence[0].Facts[0].Ref != "standard_report" {
			t.Fatal("trusted context missing")
		}
		return response, dependencyErr
	})
	p := &Participant{Access: accessStub{}, Sources: &sourceStub{current: current}, Eligibility: checker}
	got, err := p.Source(context.Background(), Actor{"1", "parent"}, association.TesteeID, association.AssessmentID.Uint64())
	if err != nil || got.Status != "ready" || got.AIEligibility != response || calls != 1 {
		t.Fatalf("source=%#v err=%v calls=%d", got, err, calls)
	}
	dependencyErr = ErrAccessUnavailable
	if _, err := p.Source(context.Background(), Actor{"1", "parent"}, association.TesteeID, association.AssessmentID.Uint64()); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatal("temporary failure disguised as missing publication")
	}
	dependencyErr = nil
	response = &Eligibility{Status: "available", ReasonCode: "private-unexpected"}
	if _, err := p.Source(context.Background(), Actor{"1", "parent"}, association.TesteeID, association.AssessmentID.Uint64()); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatal("unsafe response accepted")
	}
	denied := errors.New("revoked")
	p.Access = accessStub{denied}
	before := calls
	if _, err := p.Source(context.Background(), Actor{"1", "parent"}, association.TesteeID, association.AssessmentID.Uint64()); !errors.Is(err, denied) || calls != before {
		t.Fatal("revoked authorization reached capability query")
	}
}
