//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration && reliable_messaging_m5

package transaction

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	evaltestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	participant "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	mongoreport "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	collectiongrpc "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gorm.io/gorm"
)

// The HTTP identity and optional hint are fixtures. The original failure was
// persisted before physical broker loss; all authority reads use actual stores,
// Collection clients, signed report delegation, mTLS and production ACL.
type effectFailureReadback struct {
	db     *gorm.DB
	wait   *reportwait.Service
	bff    *grpcbridge.EvaluationBFFReader
	mu     sync.Mutex
	err    error
	checks int
}

type effectOwnedReports struct{ assessments evaltestee.Service }

func (a effectOwnedReports) AuthorizeParticipant(context.Context, participant.Actor) error {
	return nil // Isolated authenticated HTTP identity; ownership below is real.
}
func (a effectOwnedReports) AuthorizeOwnAssessment(ctx context.Context, testeeID, id uint64) error {
	return a.assessments.AuthorizeAssessment(ctx, evaltestee.Actor{TesteeID: testeeID}, id)
}

type effectFailureHint struct{ snapshot *reportstatus.Snapshot }

func (c *effectFailureHint) Get(context.Context, string) (*reportstatus.Snapshot, error) {
	return c.snapshot, nil
}
func (c *effectFailureHint) Set(_ context.Context, s *reportstatus.Snapshot, _ time.Duration) error {
	c.snapshot = s
	return nil
}
func (c *effectFailureHint) SetIfHigherPriority(ctx context.Context, s *reportstatus.Snapshot, ttl time.Duration) error {
	return c.Set(ctx, s, ttl)
}

func newEffectFailureReadback(t *testing.T, db *gorm.DB, mdb *mongo.Database, server *servergrpc.Server, ca *tlsfixture.Authority, endpoint string, gens *mongoreport.GenerationRepository, iruns *mongoreport.RunRepository) *effectFailureReadback {
	t.Helper()
	assessments := evaltestee.NewService(evaluation.NewAssessmentRepository(db), evaluation.NewAssessmentReadModel(db), nil)
	runtime := evaltestee.NewRuntimeStatusReader(assessments, checkpoint.NewRunRepository(db))
	grpcservice.NewTesteeEvaluationService(assessments, runtime).RegisterService(server.Server)
	options := &delegatedsubject.Options{Enabled: true, CurrentKey: "isolated-effect-readback-key", TTL: time.Minute}
	verifier, err := delegatedsubject.NewVerifierFromOptions(options)
	require.NoError(t, err)
	signer, err := delegatedsubject.NewSignerFromOptions(options)
	require.NoError(t, err)
	access := effectOwnedReports{assessments}
	facts := effectSQLFacts{evaluation.NewOutcomeRepository(db)}
	grpcservice.NewParticipantReportService(participant.NewService(mongoreport.NewReportReadModel(mdb), access), verifier, participant.NewRuntimeStatusReader(access, facts, gens, iruns)).RegisterService(server.Server)
	collection := ca.Issue(t, "qs-collection-server.svc", false)
	base, err := collectiongrpc.NewClient(&collectiongrpc.ClientConfig{Endpoint: endpoint, Timeout: 2 * time.Second}, grpc.WithTransportCredentials(credentials.NewTLS(ca.Client(&collection))))
	require.NoError(t, err)
	t.Cleanup(func() { _ = base.Close() })
	bff := grpcbridge.NewEvaluationBFFReader(collectiongrpc.NewTesteeEvaluationClient(base), collectiongrpc.NewParticipantReportClient(base, signer), nil)
	return &effectFailureReadback{db: db, bff: bff, wait: reportwait.NewService(bff, &effectFailureHint{}, nil, nil, reportwait.DefaultConfig())}
}

func (p *effectFailureReadback) facts(ctx context.Context) ([32]byte, error) {
	var encoded []string
	for _, table := range []string{"assessment", "runtime_checkpoint", "evaluation_outcome", "rm_outbox"} {
		var rows []map[string]any
		if err := p.db.WithContext(ctx).Table(table).Find(&rows).Error; err != nil {
			return [32]byte{}, err
		}
		for _, row := range rows {
			body, err := json.Marshal(row)
			if err != nil {
				return [32]byte{}, err
			}
			encoded = append(encoded, table+":"+string(body))
		}
	}
	sort.Strings(encoded)
	body, err := json.Marshal(encoded)
	return sha256.Sum256(body), err
}

func (p *effectFailureReadback) check(ctx context.Context, id uint64, want string, attempt int) error {
	ctx = context.WithValue(ctx, middleware.UserClaimsContextKey{}, &middleware.UserClaims{UserID: "42", OrgID: "1"})
	before, err := p.facts(ctx)
	if err != nil {
		return err
	}
	got, err := p.wait.GetStatus(ctx, 2, id)
	if err != nil || got == nil || got.Status != want {
		return fmt.Errorf("lost evaluation hint authority read: got=%+v err=%v want=%s", got, err, want)
	}
	run, err := p.bff.GetMyAssessmentRunStatus(ctx, 2, id)
	if err != nil || run == nil || run.Attempt != attempt {
		return fmt.Errorf("wrong persisted attempt: got=%+v err=%v want=%d", run, err, attempt)
	}
	if _, err = p.wait.GetStatus(ctx, 3, id); err == nil {
		return fmt.Errorf("foreign Testee could read original failure")
	}
	after, err := p.facts(ctx)
	if err != nil || before != after {
		return fmt.Errorf("authority read changed SQL business/Run/intent facts: %v", err)
	}
	p.mu.Lock()
	p.checks++
	p.mu.Unlock()
	return nil
}

func (p *effectFailureReadback) duringClaim(ctx context.Context, id uint64) {
	err := p.check(ctx, id, "processing", 2)
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
}

type effectReadbackInput struct{ probe *effectFailureReadback }

func (r *effectReadbackInput) Resolve(ctx context.Context, ref evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	r.probe.duringClaim(ctx, ref.AssessmentID)
	return (m5TerminalInput{}).Resolve(ctx, ref)
}
