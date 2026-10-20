package interpretation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/execution"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

type reportRoutes struct{}

func (reportRoutes) GetTopicForEvent(string) (string, bool) { return "qs.evaluation.lifecycle", true }

func reportTransactionRequest(t *testing.T) app.CommitSuccessRequest {
	t.Helper()
	r := runningTerminalFixture(t)
	at := r.StartedAt().Add(time.Second)
	g, err := generation.New(1, generation.Key{OutcomeID: 9, ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
	require.NoError(t, err)
	require.NoError(t, g.Begin(r.ID(), *r.StartedAt()))
	artifact, err := report.NewInterpretReport(report.InterpretReportInput{ID: 3, GenerationID: g.ID(), OutcomeID: g.Key().OutcomeID, InterpretationRunID: r.ID(),
		Association: report.Association{OrgID: 7, AssessmentID: 8, TesteeID: 9}, ReportType: g.Key().ReportType, TemplateVersion: g.Key().TemplateVersion,
		BuilderIdentity: report.BuilderIdentityFactorScoring, ContentSchemaVersion: report.ContentSchemaVersionV1, GeneratedAt: at,
		Content: report.Content{Model: report.ModelIdentity{Kind: "scale", Code: "SDS", Version: "v1", Title: "fixture"}, PrimaryScore: report.NewRawTotalScore(42, nil), Level: report.LevelFromRisk(report.RiskLevelLow), Conclusion: "first immutable content",
			Dimensions: []report.DimensionInterpret{report.NewDimensionInterpret(report.NewFactorCode("TOTAL"), "total", 42, nil, report.RiskLevelLow, "fixture", "fixture")}}})
	require.NoError(t, err)
	return app.CommitSuccessRequest{Generation: g, Run: r, InterpretReport: artifact, BuilderIdentity: artifact.BuilderIdentity(), ContentSchemaVersion: artifact.ContentSchemaVersion(), CompletedAt: at}
}

type reportBoundaryStub struct {
	plan        transaction.FrozenPlan
	local       transaction.LocalOutcome
	final       transaction.FinalOutcome
	commitCalls int
}

func (b *reportBoundaryStub) Reserve(_ context.Context, q port.Request) (transaction.Reservation, error) {
	p, err := shared.Freeze(q, reportRoutes{}, "qs.mongo", "m3-1")
	if err != nil {
		return transaction.Reservation{}, err
	}
	b.plan = p
	return transaction.Reservation{Acquired: true, Execution: b.execution()}, nil
}
func (b *reportBoundaryStub) execution() transaction.Execution {
	return transaction.Execution{Plan: b.plan, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
}
func (b *reportBoundaryStub) Find(context.Context, string, string) (transaction.Reservation, error) {
	return transaction.Reservation{}, transaction.ErrNotFound
}
func (b *reportBoundaryStub) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return shared.Artifact(p, "qs.mongo", "m3-1")
}
func (b *reportBoundaryStub) Commit(_ context.Context, e transaction.Execution, _ func(context.Context) error) (transaction.Result, error) {
	b.commitCalls++
	result := transaction.InitialResult(e)
	result.Local = b.local
	for i := range result.Branches {
		result.Branches[i].Final = b.final
	}
	if b.local != transaction.LocalCommitted {
		return result, errors.New("fixture unknown half/commit result")
	}
	return result, nil
}
func (b *reportBoundaryStub) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return transaction.LocalUnknown
}

func TestReportPlanFreezesOriginalReportAndExactBranchAcrossBSON(t *testing.T) {
	q := reportTransactionRequest(t)
	request, err := freezeReportSuccess(q)
	require.NoError(t, err)
	f, err := readReportPlan(request.Artifact)
	require.NoError(t, err)
	b := &reportBoundaryStub{}
	reservation, err := b.Reserve(t.Context(), request)
	require.NoError(t, err)
	c := &TransactionCommitter{boundary: b}
	read, err := c.readExecution(reservation.Execution)
	require.NoError(t, err)
	require.Equal(t, f.Report, read.Report)
	prepared, write, err := materializeReportCommit(read, reservation.Execution)
	require.NoError(t, err)
	require.NotNil(t, write)
	require.Equal(t, generation.StatusGenerated, prepared.Generation.Status())
	require.Equal(t, run.StatusSucceeded, prepared.Run.Status())
	require.Equal(t, "first immutable content", prepared.InterpretReport.Content().Conclusion)
	require.Nil(t, prepared.Generation.GeneratedEventEvidence())
	require.NoError(t, prepared.Generation.GeneratedTransactionEvidence().Validate())
	encoded, err := transaction.EncodePlan(reservation.Execution.Plan)
	require.NoError(t, err)
	reloaded, err := transaction.DecodePlan(encoded)
	require.NoError(t, err)
	require.Equal(t, reservation.Execution.Plan.Input().Branches[0].Wire, reloaded.Input().Branches[0].Wire)
	var po InterpretReportPO
	require.NoError(t, bson.Unmarshal(f.Report, &po))
	require.False(t, po.ID.IsZero())
	require.Equal(t, generation.StatusGenerating, q.Generation.Status())
	require.Equal(t, run.StatusRunning, q.Run.Status())
}

func TestReportFailurePlanRetainsFutureWireWithoutAnEarlyHalf(t *testing.T) {
	for _, retryable := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "automatic"}[retryable], func(t *testing.T) {
			q := reportTransactionRequest(t)
			request, err := freezeReportFailure(app.CommitFailureRequest{Generation: q.Generation, Run: q.Run, OutcomeID: q.Generation.Key().OutcomeID, Association: q.InterpretReport.Association(),
				Failure: run.Failure{Kind: run.FailureKindBuild, Code: "build_failed", SafeMessage: "failed", Retryable: retryable}, FailedAt: q.CompletedAt}, reportRoutes{})
			require.NoError(t, err)
			require.Len(t, request.Events, 1)
			require.Equal(t, catalog.InterpretationReportFailed, request.Events[0].EventType())
			b := &reportBoundaryStub{}
			reservation, err := b.Reserve(t.Context(), request)
			require.NoError(t, err)
			f, err := (&TransactionCommitter{boundary: b}).readExecution(reservation.Execution)
			require.NoError(t, err)
			prepared, _, err := materializeReportCommit(f, reservation.Execution)
			require.NoError(t, err)
			require.NoError(t, prepared.Run.FailedTransactionEvidence().Validate())
			require.Equal(t, 1, prepared.Run.Attempt())
			require.Len(t, reservation.Execution.Plan.Input().Branches, 1)
			if retryable {
				require.NotNil(t, f.Future)
				require.True(t, f.Due.After(f.At))
				require.Equal(t, prepared.Run.RetryDecision().RetryEventID, f.Future.ID)
				require.Equal(t, catalog.InterpretationRetryRequested, f.Future.EventType)
				require.NotNil(t, prepared.Run.RetryEventEvidence())
			} else {
				require.Nil(t, f.Future)
				require.True(t, f.Due.IsZero())
			}
		})
	}
}

func TestReportPlanRejectsBodySnapshotAndRootIdentitySubstitution(t *testing.T) {
	q := reportTransactionRequest(t)
	request, err := freezeReportSuccess(q)
	require.NoError(t, err)
	for _, field := range []string{"body", "artifact", "claim", "noncanonical"} {
		t.Run(field, func(t *testing.T) {
			f, err := readReportPlan(request.Artifact)
			require.NoError(t, err)
			switch field {
			case "body":
				f.Body = bytes.ReplaceAll(f.Body, []byte(`"org_id":7`), []byte(`"org_id":8`))
			case "artifact":
				var po InterpretReportPO
				require.NoError(t, bson.Unmarshal(f.Report, &po))
				po.GenerationID++
				f.Report, err = bson.Marshal(po)
				require.NoError(t, err)
			case "claim":
				var po InterpretationRunPO
				require.NoError(t, bson.Unmarshal(f.Original, &po))
				po.GenerationID++
				f.Original, err = bson.Marshal(po)
				require.NoError(t, err)
			}
			raw, err := json.Marshal(f)
			require.NoError(t, err)
			if field == "noncanonical" {
				raw = append(raw, '\n')
			}
			_, err = readReportPlan(raw)
			require.Error(t, err)
		})
	}
}

func TestReportSenderKeepsLocalCommitSeparateFromFinalConfirmation(t *testing.T) {
	for _, local := range []transaction.LocalOutcome{transaction.LocalNotStarted, transaction.LocalUnknown, transaction.LocalAborted, transaction.LocalCommitted} {
		t.Run(string(local), func(t *testing.T) {
			b := &reportBoundaryStub{local: local, final: transaction.FinalUnknown}
			c := &TransactionCommitter{boundary: b, resolver: reportRoutes{}}
			result, err := c.CommitSuccess(t.Context(), reportTransactionRequest(t))
			require.Equal(t, 1, b.commitCalls)
			if local == transaction.LocalCommitted {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.ErrorIs(t, err, port.ErrUnresolved)
				require.Nil(t, result)
			}
		})
	}
}

func TestReportReadOnlyRecoveryDoesNotRequireOrRecreateWinnerCapability(t *testing.T) {
	request, err := freezeReportSuccess(reportTransactionRequest(t))
	require.NoError(t, err)
	b := &reportBoundaryStub{}
	r, err := b.Reserve(t.Context(), request)
	require.NoError(t, err)
	f, err := (&TransactionCommitter{boundary: b}).readExecution(r.Execution)
	require.NoError(t, err)
	winner, err := reportPlanProof(f, r.Execution)
	require.NoError(t, err)
	readOnly := r.Execution
	readOnly.Token = [32]byte{}
	require.False(t, readOnly.Valid(), "read-only lookup must not acquire an execution capability")
	proof, err := reportPlanProof(f, readOnly)
	require.NoError(t, err)
	require.True(t, sameTransactionProof(winner, proof))
	prepared, _, err := materializeReportCommit(f, readOnly)
	require.NoError(t, err)
	require.Equal(t, generation.StatusGenerated, prepared.Generation.Status())
	require.Zero(t, b.commitCalls)
	require.Equal(t, [32]byte{}, readOnly.Token)
}

func TestReportFutureOwnershipVersionKeepsLegacyRequestAndOriginalWire(t *testing.T) {
	q := reportTransactionRequest(t)
	request, err := freezeReportFailure(app.CommitFailureRequest{Generation: q.Generation, Run: q.Run, OutcomeID: q.Generation.Key().OutcomeID, Association: q.InterpretReport.Association(), Failure: run.Failure{Kind: run.FailureKindBuild, Code: "build_failed", SafeMessage: "controlled", Retryable: true}, FailedAt: q.CompletedAt}, reportRoutes{})
	require.NoError(t, err)
	current, err := readReportPlan(request.Artifact)
	require.NoError(t, err)
	require.Equal(t, 2, current.Version)
	plan, err := current.retryPlan()
	require.NoError(t, err)
	require.Equal(t, current.Future.Payload, plan.Message().Input().Payload)
	_, _, terminal, _, err := current.records()
	require.NoError(t, err)
	require.NoError(t, plan.MatchDecision(terminal.RetryDecision(), current.Due.Truncate(time.Millisecond)))
	legacy := current
	legacy.Version = 1
	raw, err := json.Marshal(legacy)
	require.NoError(t, err)
	old, err := readReportPlan(raw)
	require.NoError(t, err)
	first, err := old.inputHash()
	require.NoError(t, err)
	latest, err := current.inputHash()
	require.NoError(t, err)
	require.Equal(t, first, latest)
	_, err = old.retryPlan()
	require.ErrorIs(t, err, transaction.ErrInvalidPlan)
	require.Equal(t, current.Future.Payload, old.Future.Payload)
}
