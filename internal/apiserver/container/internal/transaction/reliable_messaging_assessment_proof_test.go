//go:build reliable_messaging

package transaction

import (
	"context"
	"os"
	"testing"
	"time"

	intake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	persistence "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	oldoutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Characterization of the original Assessment persistence boundary, not an SDK
// consumer acceptance test. Model validation is a fixture; SQL, repository,
// transaction runner, intake service and historical Outbox are real.
func TestReliableMessagingAssessmentPersistence(t *testing.T) {
	dsn := os.Getenv("RM_QS_ASSESSMENT_DSN")
	if dsn == "" {
		t.Fatal("isolated MySQL DSN required; must not skip")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(driver.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	// Current PO schema fixture, not a production migration acceptance claim.
	require.NoError(t, db.AutoMigrate(&persistence.AssessmentPO{}, &oldoutbox.OutboxPO{}))
	config, err := catalog.Parse([]byte(`version: "1"
topics:
  proof:
    name: qs.rm.assessment
events:
  evaluation.requested:
    topic: proof
    delivery: durable_outbox
    aggregate: Assessment
    domain: evaluation
    handler: proof
`))
	require.NoError(t, err)
	repo := persistence.NewAssessmentRepository(db)
	runner := NewMySQLRunner(db)
	stager := oldoutbox.NewStoreWithTopicResolver(db, catalog.NewCatalog(config))
	service := intake.NewService(repo, proofModelValidator{}, runner, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	command := intake.CreateCommand{OrgID: 1, TesteeID: 2, AnswerSheetID: 3, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version}
	created, err := service.CreateForAnswerSheet(ctx, command)
	require.NoError(t, err)
	require.Equal(t, "pending", created.Status)
	_, err = service.CreateForAnswerSheet(ctx, command)
	require.Error(t, err, "unique answer-sheet identity must reject duplicate creation")
	found, err := service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, created.ID, found.ID)
	countEvents := func() int64 {
		var n int64
		require.NoError(t, db.WithContext(ctx).Model(&oldoutbox.OutboxPO{}).Where("event_type = ?", "evaluation.requested").Count(&n).Error)
		return n
	}
	require.Zero(t, countEvents())
	// A real server-side insert fault must roll back the status transition too.
	require.NoError(t, db.Exec("CREATE TRIGGER rm_reject_assessment_event BEFORE INSERT ON domain_event_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'proof event write failure'").Error)
	_, err = service.SubmitForEvaluation(ctx, created.ID)
	require.Error(t, err)
	found, err = service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "pending", found.Status)
	require.Zero(t, countEvents())
	require.NoError(t, db.Exec("DROP TRIGGER rm_reject_assessment_event").Error)

	// Both requests read pending before either commits. The wrapper only places
	// a deterministic scheduling barrier after the real repository read.
	barrier := &proofPendingBarrier{Repository: repo, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	concurrent := intake.NewService(barrier, proofModelValidator{}, runner, stager)
	results := make(chan error, 2)
	for range 2 {
		go func() { _, e := concurrent.SubmitForEvaluation(ctx, created.ID); results <- e }()
	}
	for range 2 {
		select {
		case <-barrier.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(barrier.release)
	for range 2 {
		require.NoError(t, <-results)
	}
	// Known baseline gap: two independent event identities are persisted for one
	// business transition. This passing characterization is NOT an idempotency pass.
	require.EqualValues(t, 2, countEvents(), "update characterization if host CAS protection is introduced")
	found, err = service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "submitted", found.Status)
	_, err = service.SubmitForEvaluation(ctx, created.ID)
	require.Error(t, err, "sequential replay sees submitted and rejects a second transition")
	require.EqualValues(t, 2, countEvents())
	var assessments int64
	require.NoError(t, db.Model(&persistence.AssessmentPO{}).Count(&assessments).Error)
	require.EqualValues(t, 1, assessments)
	t.Log("baseline gap reproduced: one assessment, two evaluation.requested rows after concurrent pending reads; M2 consumer gate remains open")
}

type proofModelValidator struct{}

func (proofModelValidator) ValidateEvaluationModel(context.Context, domain.EvaluationModelRef, domain.QuestionnaireRef, intake.ModelValidationMode) error {
	return nil
}

type proofPendingBarrier struct {
	domain.Repository
	arrived chan struct{}
	release chan struct{}
}

func (r *proofPendingBarrier) FindByID(ctx context.Context, id domain.ID) (*domain.Assessment, error) {
	item, err := r.Repository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	select {
	case r.arrived <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-r.release:
		return item, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
