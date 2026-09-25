//go:build integration && reliable_messaging_m4 && reliable_messaging_m5

package runtimeclosure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	collectionclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	mysqlunit "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/admission"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type m5UnavailableRuntimeInput struct{}

type m5UnavailableStatusCache struct{}

func (m5UnavailableStatusCache) Get(context.Context, string) (*reportstatus.Snapshot, error) {
	return nil, errors.New("controlled status cache unavailable")
}

func (m5UnavailableStatusCache) Set(context.Context, *reportstatus.Snapshot, time.Duration) error {
	return errors.New("controlled status cache unavailable")
}

func (m5UnavailableStatusCache) SetIfHigherPriority(context.Context, *reportstatus.Snapshot, time.Duration) error {
	return errors.New("controlled status cache unavailable")
}

func (m5UnavailableRuntimeInput) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, evaluationinput.NewDependencyResolveError(
		evaluationinput.DependencyCategoryModelCatalog, errors.New("controlled unavailable input"),
		"controlled unavailable input", "controlled unavailable input",
	)
}

// A physically redelivered old failure cannot hide a newer durable report
// from the participant, even if it temporarily replaces the Redis projection.
func TestM5OldFailureRedeliveryAfterDurableReportKeepsParticipantCompleted(t *testing.T) {
	if os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	originalBusiness, originalOutbox := retrygovernance.BusinessPolicy(), retrygovernance.OutboxPolicy()
	fast := originalBusiness
	fast.Version += "-ordering-probe"
	fast.BaseDelay, fast.MaxDelay, fast.JitterFraction = 2*time.Second, 2*time.Second, 0
	require.NoError(t, retrygovernance.ConfigurePolicies(fast, originalOutbox))
	t.Cleanup(func() { require.NoError(t, retrygovernance.ConfigurePolicies(originalBusiness, originalOutbox)) })

	scenario := runtimeClosureScenario{
		skipReportWaitClosure: true,
		beforeEvaluation: func(t *testing.T, assessmentID uint64, db *gorm.DB, events *subsystem.Subsystem) {
			binding := events.Profile(eventcatalog.OutboxProfileAssessmentMySQL)
			require.NotNil(t, binding.Stager)
			repo := assessmentmysql.NewAssessmentRepository(db)
			runs := checkpoint.NewRunRepository(db)
			uow := mysqlunit.NewUnitOfWork(db)
			runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
				return uow.WithinTransaction(ctx, fn)
			})
			engine := appexecute.NewEngine(repo, m5UnavailableRuntimeInput{},
				appexecute.WithRunRepository(runs),
				appexecute.WithTransactionalOutbox(runner, binding.Stager),
				appexecute.WithPostCommitDispatcher(binding.PostCommit),
			)
			require.Error(t, engine.Evaluate(t.Context(), assessmentID))
			latest, err := runs.FindLatestByAssessmentID(t.Context(), assessmentID)
			require.NoError(t, err)
			require.NotNil(t, latest)
			require.Equal(t, 1, latest.Attempt().Number)
			require.Equal(t, "failed", latest.Attempt().Status.String())
			var failedCount int64
			require.NoError(t, db.Table("rm_outbox").Where("event_type = ?", eventcatalog.EvaluationFailed).Count(&failedCount).Error)
			require.EqualValues(t, 1, failedCount)
		},
		afterReport: func(t *testing.T, delivery runtimeClosureDelivery, deps grpctransport.Deps, testeeID, assessmentID uint64) {
			query := newM5RuntimeStatusGRPCReader(t, deps)
			run, err := query.GetMyAssessmentRunStatus(t.Context(), testeeID, assessmentID)
			require.NoError(t, err)
			require.NotNil(t, run)
			require.Equal(t, 2, run.Attempt)
			require.Equal(t, "succeeded", run.Status)
			_, err = query.GetMyAssessmentRunStatus(t.Context(), testeeID+1, assessmentID)
			require.Equal(t, codes.PermissionDenied, status.Code(err), "the participant may read only their own latest Run")
			report, err := query.GetAssessmentReport(t.Context(), testeeID, assessmentID)
			require.NoError(t, err)
			require.NotNil(t, report, "the newer report must be a real Mongo fact")
			cache := deps.Interpretation.ReportStatusReporter.Cache()
			snapshot, err := cache.Get(t.Context(), strconv.FormatUint(assessmentID, 10))
			require.NoError(t, err)
			require.Equal(t, "completed", snapshot.Status)
			require.NotEmpty(t, snapshot.ReportID)
			reportVerifiedAt := time.Now()

			oldFailure, err := delivery.Wait(t, eventcatalog.EvaluationFailed)
			require.NoError(t, err)
			require.NotEmpty(t, oldFailure.UUID)
			physical, ok := delivery.(*standardClosureDelivery)
			require.True(t, ok)
			require.Greater(t, physical.lastFailureAttempts, uint16(1))
			require.True(t, physical.lastFailureHandledAt.After(reportVerifiedAt),
				"old failure must be physically handled after the newer report is durable")
			t.Logf("old failure event=%s broker=%s attempts=%d after report=%s", oldFailure.UUID,
				physical.firstFailureBrokerID, physical.lastFailureAttempts, reportVerifiedAt.Format(time.RFC3339Nano))
			runAfterRedelivery, err := query.GetMyAssessmentRunStatus(t.Context(), testeeID, assessmentID)
			require.NoError(t, err)
			require.Equal(t, run, runAfterRedelivery, "old failure delivery must not change the durable latest Run")

			waiter := reportwait.NewService(query, cache, nil, nil, reportwait.DefaultConfig())
			visible, err := waiter.GetStatus(t.Context(), testeeID, assessmentID)
			require.NoError(t, err)
			require.Equal(t, "completed", visible.Status,
				fmt.Sprintf("real report is durable but old failure redelivery changed Collection status: %+v", visible))

			unavailableWaiter := reportwait.NewService(query, m5UnavailableStatusCache{}, nil, nil, reportwait.DefaultConfig())
			visibleWithoutCache, err := unavailableWaiter.GetStatus(t.Context(), testeeID, assessmentID)
			require.NoError(t, err)
			require.Equal(t, "completed", visibleWithoutCache.Status,
				"a cache read error after old failure redelivery must not hide the durable report")
			t.Logf("cache read error after old failure redelivery: participant status=%s", visibleWithoutCache.Status)
		},
	}
	runCurrentRuntimeClosure(t, func(t *testing.T, opts subsystem.Options, db *sql.DB) (*subsystem.Subsystem, runtimeClosureDelivery, error) {
		return newM5StandardEventSubsystem(t, opts, db, true, true)
	}, scenario)
}

func newM5RuntimeStatusGRPCReader(t *testing.T, deps grpctransport.Deps) *grpcbridge.EvaluationBFFReader {
	t.Helper()
	ca := tlsfixture.New(t)
	serverPair := ca.Issue(t, "server.test", false)
	srv, err := servergrpc.NewServer(&servergrpc.Config{
		TLSCertFile: serverPair.CertFile, TLSKeyFile: serverPair.KeyFile,
		MTLS: servergrpc.MTLSConfig{Enabled: true, CAFile: ca.CAFile, RequireClientCert: true},
		ACL:  servergrpc.ACLConfig{Enabled: true, ConfigFile: "../../../../configs/grpc-acl.prod.yaml", DefaultPolicy: "deny"},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(srv.Stop)
	grpcservice.NewTesteeEvaluationService(deps.Evaluation.TesteeService, deps.Evaluation.RuntimeStatusReader).RegisterService(srv.Server)
	grpcservice.NewParticipantReportService(deps.Interpretation.ParticipantService, deps.Interpretation.DelegatedSubjectVerifier).RegisterService(srv.Server)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	go func() { _ = srv.Serve(lis) }()

	collectionPair := ca.Issue(t, "qs-collection-server.svc", false)
	manager, err := collectionclient.NewManager(&collectionclient.ManagerConfig{
		Endpoint: lis.Addr().String(), Timeout: time.Second,
		InflightSemaphore: admission.NewChannelSemaphore(2),
		TLSCertFile:       collectionPair.CertFile, TLSKeyFile: collectionPair.KeyFile,
		TLSCAFile: ca.CAFile, TLSServerName: "server.test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	require.NoError(t, manager.RegisterClients())
	return grpcbridge.NewEvaluationBFFReader(manager.TesteeEvaluationClient(), manager.ParticipantReportClient(), nil)
}
