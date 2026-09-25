package grpc_test

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	domaintestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	collection "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/admission"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type runtimeStatusAssessmentRepo struct {
	domainassessment.Repository
	assessment *domainassessment.Assessment
}

func (r runtimeStatusAssessmentRepo) FindByID(_ context.Context, id domainassessment.ID) (*domainassessment.Assessment, error) {
	if id != r.assessment.ID() {
		return nil, errors.New("assessment not found")
	}
	return r.assessment, nil
}

type runtimeStatusRunRepo struct {
	run   *evalrun.EvaluationRun
	reads atomic.Int32
}

func (r *runtimeStatusRunRepo) FindLatestByAssessmentID(_ context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error) {
	r.reads.Add(1)
	if assessmentID != r.run.AssessmentID() {
		return nil, nil
	}
	return r.run, nil
}

// This exercises the Collection client, production ACL and mTLS interceptor
// against the real participant ownership reader over a TCP gRPC connection.
func TestCollectionRuntimeStatusMTLSAndOwnership(t *testing.T) {
	ca := tlsfixture.New(t)
	serverPair := ca.Issue(t, "server.test", false)
	cfg := &servergrpc.Config{
		TLSCertFile: serverPair.CertFile, TLSKeyFile: serverPair.KeyFile,
		MTLS: servergrpc.MTLSConfig{Enabled: true, CAFile: ca.CAFile, RequireClientCert: true},
		ACL:  servergrpc.ACLConfig{Enabled: true, ConfigFile: "../../../configs/grpc-acl.prod.yaml", DefaultPolicy: "deny"},
	}
	srv, err := servergrpc.NewServer(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(srv.Stop)

	assessment, err := domainassessment.NewAssessment(9, domaintestee.NewID(7),
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q"), "1"),
		domainassessment.NewAnswerSheetRef(meta.FromUint64(2)),
		domainassessment.NewAdhocOrigin(), domainassessment.WithID(meta.FromUint64(42)))
	require.NoError(t, err)
	run := evalrun.NewEvaluationRunWithAttempt(42, 2)
	require.NoError(t, run.Start(time.Now()))
	runs := &runtimeStatusRunRepo{run: &run}
	access := evaluationtestee.NewService(runtimeStatusAssessmentRepo{assessment: assessment}, nil, nil)
	grpcservice.NewTesteeEvaluationService(access, evaluationtestee.NewRuntimeStatusReader(access, runs)).RegisterService(srv.Server)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	go func() { _ = srv.Serve(lis) }()

	valid := ca.Issue(t, "qs-collection-server.svc", false)
	manager, err := collection.NewManager(&collection.ManagerConfig{
		Endpoint: lis.Addr().String(), Timeout: time.Second,
		InflightSemaphore: admission.NewChannelSemaphore(2),
		TLSCertFile:       valid.CertFile, TLSKeyFile: valid.KeyFile,
		TLSCAFile: ca.CAFile, TLSServerName: "server.test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	require.NoError(t, manager.RegisterClients())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := manager.TesteeEvaluationClient().GetMyAssessmentRunStatus(ctx, 7, 42)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 2, got.Attempt)
	require.Equal(t, "running", got.Status)
	require.EqualValues(t, 1, runs.reads.Load())

	_, err = manager.TesteeEvaluationClient().GetMyAssessmentRunStatus(ctx, 8, 42)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.EqualValues(t, 1, runs.reads.Load(), "ownership must be checked before reading a run")

	unknown := ca.Issue(t, "unknown.svc", false)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(ca.Client(&unknown))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = evaluationpb.NewTesteeEvaluationServiceClient(conn).GetMyAssessmentRunStatus(ctx,
		&evaluationpb.GetMyAssessmentRunStatusRequest{TesteeId: 7, AssessmentId: 42})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.EqualValues(t, 1, runs.reads.Load(), "ACL must reject unknown services before reading a run")
}
