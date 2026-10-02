package participant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	irun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type runtimeFactReader struct {
	fact  *evaluationfact.Record
	calls int
	err   error
}

func (r *runtimeFactReader) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	panic("unexpected unfenced outcome lookup")
}
func (r *runtimeFactReader) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	r.calls++
	return r.fact, r.err
}

type runtimeLifecycleReader struct {
	current, fresh *generation.ReportGeneration
	run            *irun.InterpretationRun
	keys           []generation.Key
	calls          int
	change         bool
}

func (r *runtimeLifecycleReader) FindByKey(_ context.Context, k generation.Key) (*generation.ReportGeneration, error) {
	r.keys = append(r.keys, k)
	return r.current, nil
}
func (r *runtimeLifecycleReader) FindByID(_ context.Context, id meta.ID) (*generation.ReportGeneration, error) {
	r.calls++
	if r.change {
		r.current = r.fresh
		return r.fresh, nil
	}
	if r.fresh != nil {
		return r.fresh, nil
	}
	return r.current, nil
}

type runtimeRunReader struct {
	owner  *runtimeLifecycleReader
	latest *irun.InterpretationRun
	calls  int
}

func (r *runtimeRunReader) FindByID(_ context.Context, id meta.ID) (*irun.InterpretationRun, error) {
	r.calls++
	if r.latest != nil && id == r.latest.ID() {
		return r.latest, nil
	}
	return r.owner.run, nil
}

func frozenRuntimeFact(t *testing.T) *evaluationfact.Record {
	t.Helper()
	routing := &evaluationinput.TypologyRoutingFreeze{DecisionKind: string(modelcatalog.DecisionKindPoleComposition), ReportKind: "template", AdapterKey: "personality_type", TemplateID: "personality", TemplateVersion: "frozen-v3"}
	assets := &interpretationassets.Assets{
		Outcomes:   []interpretationassets.OutcomePresentation{{OutcomeCode: "INTJ", Title: "建筑师", Summary: "冻结摘要"}},
		Profiles:   []interpretationassets.TypeProfilePresentation{{OutcomeCode: "INTJ", Commentary: "冻结摘要"}},
		ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{Code: "personality", Kind: routing.ReportKind, AdapterKey: routing.AdapterKey, TemplateID: routing.TemplateID, TemplateVersion: routing.TemplateVersion}}},
	}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{Assets: assets, ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "PERSONALITY", Version: "1.0.0"}, DecisionKind: modelcatalog.DecisionKindPoleComposition, TypologyRouting: routing})
	if err != nil {
		t.Fatal(err)
	}
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(20), OrgID: 1, AssessmentID: meta.FromUint64(10), TesteeID: 2, RunID: "10:1", Model: evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "PERSONALITY", Version: "1.0.0"}, Runtime: evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindPoleComposition}, SchemaVersion: 2, EvaluatedAt: time.Unix(100, 0), ReportInput: raw, Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80}},"Primary":{"Kind":"match_percent","Value":80,"Label":"INTJ"},"Profile":{"Kind":"personality_type","Code":"INTJ"}}`)})
}
func lifecycleRuntimeFixture(t *testing.T, state irun.Status, disposition retrygovernance.Disposition, retryable bool, attempt int) (*generation.ReportGeneration, *irun.InterpretationRun) {
	t.Helper()
	now := time.Unix(100, 0)
	end := now.Add(time.Second)
	key := generation.Key{OutcomeID: meta.FromUint64(20), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("frozen-v3")}
	rid := meta.FromUint64(uint64(100 + attempt))
	gid := meta.FromUint64(30)
	ri := irun.RestoreInput{ID: rid, GenerationID: gid, Attempt: attempt, Status: state}
	gi := generation.RestoreInput{ID: gid, Key: key, LatestRunID: rid, Status: generation.StatusGenerating, Version: uint64(attempt + 1), CreatedAt: now, UpdatedAt: end}
	switch state {
	case irun.StatusRunning:
		ri.StartedAt = &now
	case irun.StatusSucceeded:
		ri.StartedAt = &now
		ri.FinishedAt = &end
		gi.Status = generation.StatusGenerated
		gi.ReportID = meta.FromUint64(40)
	case irun.StatusFailed:
		ri.StartedAt = &now
		ri.FinishedAt = &end
		ri.Failure = &irun.Failure{Kind: irun.FailureKindBuild, Code: "private_error", SafeMessage: "private details must not be returned", Retryable: retryable}
		gi.Status = generation.StatusFailed
		if disposition != "" {
			ri.RetryDecision = &retrygovernance.Decision{Disposition: disposition}
		}
	}
	g, err := generation.Restore(gi)
	if err != nil {
		t.Fatal(err)
	}
	r, err := irun.Restore(ri)
	if err != nil {
		t.Fatal(err)
	}
	return g, r
}
func TestRuntimeStatusOwnershipBeforeAnyFactRead(t *testing.T) {
	facts := &runtimeFactReader{}
	states := &runtimeLifecycleReader{}
	runs := &runtimeRunReader{owner: states}
	denied := errors.New("denied")
	_, err := NewRuntimeStatusReader(accessStub{err: denied}, facts, states, runs).Get(t.Context(), Actor{TesteeID: 2}, 10)
	if !errors.Is(err, denied) || facts.calls != 0 || len(states.keys) != 0 || runs.calls != 0 {
		t.Fatalf("ownership guard err=%v reads=%d/%d/%d", err, facts.calls, len(states.keys), runs.calls)
	}
}
func TestRuntimeStatusUsesFrozenGenerationAndCurrentRetryDisposition(t *testing.T) {
	for _, tc := range []struct {
		name              string
		state             irun.Status
		disposition, want retrygovernance.Disposition
		retryable         bool
	}{
		{"manual", irun.StatusFailed, retrygovernance.DispositionManualRequired, retrygovernance.DispositionManualRequired, true},
		{"terminal", irun.StatusFailed, retrygovernance.DispositionTerminal, retrygovernance.DispositionTerminal, true},
		{"automatic", irun.StatusFailed, retrygovernance.DispositionAutomatic, retrygovernance.DispositionAutomatic, false},
		{"legacy_retryable", irun.StatusFailed, "", retrygovernance.DispositionAutomatic, true},
		{"legacy_terminal", irun.StatusFailed, "", retrygovernance.DispositionTerminal, false},
		{"new_pending", irun.StatusPending, "", "", false},
		{"running", irun.StatusRunning, "", "", false},
		{"succeeded_not_report", irun.StatusSucceeded, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, r := lifecycleRuntimeFixture(t, tc.state, tc.disposition, tc.retryable, 2)
			facts := &runtimeFactReader{fact: frozenRuntimeFact(t)}
			states := &runtimeLifecycleReader{current: g, run: r}
			runs := &runtimeRunReader{owner: states}
			got, err := NewRuntimeStatusReader(accessStub{}, facts, states, runs).Get(t.Context(), Actor{TesteeID: 2}, 10)
			if err != nil || got == nil || got.Status != string(tc.state) || got.Attempt != 2 || got.RetryDisposition != tc.want {
				t.Fatalf("got=%+v err=%v", got, err)
			}
			if len(states.keys) != 1 || states.keys[0].TemplateVersion.String() != "frozen-v3" || states.keys[0].OutcomeID.Uint64() != 20 || facts.calls != 2 {
				t.Fatalf("wrong frozen key or fence reads=%d keys=%v", facts.calls, states.keys)
			}
		})
	}
}
func TestRuntimeStatusRetryDuringReadCannotReturnOldTerminal(t *testing.T) {
	old, oldRun := lifecycleRuntimeFixture(t, irun.StatusFailed, retrygovernance.DispositionManualRequired, true, 1)
	newer, newRun := lifecycleRuntimeFixture(t, irun.StatusPending, "", false, 2)
	states := &runtimeLifecycleReader{current: old, fresh: newer, run: oldRun, change: true}
	runs := &runtimeRunReader{owner: states, latest: newRun}
	got, err := NewRuntimeStatusReader(accessStub{}, &runtimeFactReader{fact: frozenRuntimeFact(t)}, states, runs).Get(t.Context(), Actor{TesteeID: 2}, 10)
	if err != nil || got == nil || got.Status != "pending" || got.Attempt != 2 || len(states.keys) != 2 {
		t.Fatalf("stale terminal escaped: got=%+v err=%v reads=%d", got, err, len(states.keys))
	}
}
func TestRuntimeStatusRejectsWrongLatestRunAndPropagatesStorageFailure(t *testing.T) {
	g, r := lifecycleRuntimeFixture(t, irun.StatusFailed, retrygovernance.DispositionTerminal, false, 1)
	_, other := lifecycleRuntimeFixture(t, irun.StatusPending, "", false, 2)
	states := &runtimeLifecycleReader{current: g, run: other}
	facts := &runtimeFactReader{fact: frozenRuntimeFact(t)}
	if _, err := NewRuntimeStatusReader(accessStub{}, facts, states, &runtimeRunReader{owner: states}).Get(t.Context(), Actor{TesteeID: 2}, 10); err == nil {
		t.Fatal("wrong latest run accepted")
	}
	states.run = r
	unavailable := errors.New("storage unavailable")
	facts.err = unavailable
	if _, err := NewRuntimeStatusReader(accessStub{}, facts, states, &runtimeRunReader{owner: states}).Get(t.Context(), Actor{TesteeID: 2}, 10); !errors.Is(err, unavailable) {
		t.Fatalf("storage error=%v", err)
	}
}
