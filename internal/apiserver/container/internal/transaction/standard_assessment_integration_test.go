//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"os"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	errorcode "github.com/FangcunMount/qs-server/internal/pkg/code"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStandardAssessmentOriginalTransaction(t *testing.T) {
	dsn := os.Getenv("RM_QS_ASSESSMENT_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_assessment" {
		t.Fatal("disposable m4_qs_assessment MySQL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Assessment
    domain: evaluation
    handler: evaluation_requested_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	repo := assessmentcache.NewInvalidatingAssessmentRepository(assessmentmysql.NewAssessmentRepository(db), nil)
	runner := NewMySQLRunner(db)
	service := appintake.NewService(repo, proofModelValidator{}, runner, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	command := appintake.CreateCommand{OrgID: 1, TesteeID: 2, AnswerSheetID: 3, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version}
	created, err := service.CreateForAnswerSheet(ctx, command)
	require.NoError(t, err)
	require.Equal(t, "pending", created.Status)
	var n int64
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.Zero(t, n)

	// The database rejects the intent after the domain state update; the host
	// transaction must roll the Assessment back to pending.
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_standard_event BEFORE INSERT ON rm_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'proof standard event write failure'`).Error)
	_, err = service.SubmitForEvaluation(ctx, created.ID)
	require.Error(t, err)
	found, err := service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "pending", found.Status)
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Exec("DROP TRIGGER rm_reject_standard_event").Error)

	barrier := &proofPendingBarrier{Repository: repo, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	concurrent := appintake.NewService(barrier, proofModelValidator{}, runner, stager)
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
	var successes int
	for range 2 {
		if resultErr := <-results; resultErr == nil {
			successes++
		} else {
			require.True(t, cberrors.IsCode(resultErr, errorcode.ErrConflict), "unexpected competing error: %v", resultErr)
		}
	}
	require.Equal(t, 1, successes)
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.EqualValues(t, 1, n)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 2, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, "evaluation.requested", claims[0].Message.Input().EventType)
	require.Equal(t, "org:1", claims[0].Message.Input().Scope)
	require.Equal(t, "qs.evaluation.lifecycle", claims[0].Message.Input().Destination)
	require.NoError(t, store.Confirm(ctx, claims[0]))
	found, err = service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "submitted", found.Status)
}
