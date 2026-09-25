//go:build reliable_messaging_m4

package plan_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	appplan "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	mysqlplan "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type rejectOpenedIntentStager struct{}

func (rejectOpenedIntentStager) Stage(ctx context.Context, _ ...event.DomainEvent) error {
	if _, err := mysql.RequireTx(ctx); err != nil {
		return err
	}
	return errors.New("reject reminder intent after task update")
}

func TestTaskOpeningAndReminderIntentCommitTogetherMySQL(t *testing.T) {
	dsn := os.Getenv("QS_M5_TASK_OPEN_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_M5_TASK_OPEN_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_M5_TASK_OPEN_MYSQL_DSN required")
		}
		t.Skip("disposable MySQL not configured")
	}
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_m5_task_open_test_"), "disposable database required")
	require.NoError(t, db.AutoMigrate(&mysqlplan.AssessmentTaskPO{}))
	_, err = sqlDB.ExecContext(t.Context(), sdkmysql.Schema)
	require.NoError(t, err)

	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  task-lifecycle:
    name: qs.plan.task
events:
  task.opened.reminder.requested:
    topic: task-lifecycle
    delivery: durable_outbox
    aggregate: AssessmentTask
    domain: plan
    handler: task_opened_reminder_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	repository := mysqlplan.NewTaskRepository(db)
	uow := mysql.NewUnitOfWork(db)
	runner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return uow.WithinTransaction(ctx, fn)
	})
	legacyPublisher := &openEventRecorder{}
	service := appplan.NewTaskManagementServiceWithEnrollment(
		repository, nil, nil, runner, &uniqueOpenEntryGenerator{}, legacyPublisher,
		appEventing.ProfileBinding{Stager: stager},
	)
	newTask := func(sequence int) *domainplan.AssessmentTask {
		plannedAt := time.Now().Add(-time.Minute).UTC()
		task := domainplan.NewAssessmentTaskAt(domainplan.NewAssessmentPlanID(), sequence, 501, testee.NewID(21), "scale", plannedAt, plannedAt)
		require.NoError(t, repository.Save(t.Context(), task))
		return task
	}
	task := newTask(2)
	var initialRows int64
	require.NoError(t, db.Table("rm_outbox").Count(&initialRows).Error)
	assertPendingWithoutIntent := func() {
		t.Helper()
		found, findErr := repository.FindByID(t.Context(), task.GetID())
		require.NoError(t, findErr)
		require.Equal(t, domainplan.TaskStatusPending, found.GetStatus())
		var rows int64
		require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
		require.Equal(t, initialRows, rows)
		require.Empty(t, legacyPublisher.events)
	}

	// Stage failure after the real MySQL task update must roll that update back.
	failedService := appplan.NewTaskManagementServiceWithEnrollment(
		repository, nil, nil, runner, &uniqueOpenEntryGenerator{}, legacyPublisher,
		appEventing.ProfileBinding{Stager: rejectOpenedIntentStager{}},
	)
	_, err = failedService.OpenTask(t.Context(), 501, task.GetID().String())
	require.Error(t, err)
	assertPendingWithoutIntent()

	// An outer REST command transaction owns both writes; the inner Plan
	// operation must not create an independently committed reminder.
	rolledBack := errors.New("outer command rollback")
	err = runner.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, openErr := service.OpenTask(txCtx, 501, task.GetID().String())
		if openErr != nil {
			return openErr
		}
		return rolledBack
	})
	require.ErrorIs(t, err, rolledBack)
	assertPendingWithoutIntent()

	_, err = service.OpenTask(t.Context(), 501, task.GetID().String())
	require.NoError(t, err)
	found, err := repository.FindByID(t.Context(), task.GetID())
	require.NoError(t, err)
	require.Equal(t, domainplan.TaskStatusOpened, found.GetStatus())
	require.Empty(t, legacyPublisher.events, "a staged opening must not also direct-publish")
	var rows int64
	require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
	require.Equal(t, initialRows+1, rows)
	var rawID, rawDestination, rawScope, rawPayload []byte
	require.NoError(t, db.Raw(`SELECT message_id,destination,scope,payload FROM rm_outbox WHERE event_type=? ORDER BY id DESC LIMIT 1`, domainplan.EventTypeTaskOpenedReminderRequested).
		Row().Scan(&rawID, &rawDestination, &rawScope, &rawPayload))
	require.NotEmpty(t, rawID)
	require.Equal(t, "qs.plan.task", string(rawDestination))
	require.Equal(t, "org:501", string(rawScope))
	decoded, recognized, err := messaging.DecodeMessagePayload(rawPayload)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, string(rawID), decoded.UUID)
	var wire struct {
		ID        string `json:"id"`
		EventType string `json:"eventType"`
		Data      struct {
			TaskID           string `json:"task_id"`
			OrgID            int64  `json:"org_id"`
			ScheduleRevision uint32 `json:"schedule_revision"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(decoded.Payload, &wire))
	require.Equal(t, string(rawID), wire.ID)
	require.Equal(t, domainplan.EventTypeTaskOpenedReminderRequested, wire.EventType)
	require.Equal(t, task.GetID().String(), wire.Data.TaskID)
	require.EqualValues(t, 501, wire.Data.OrgID)
	require.Equal(t, found.GetScheduleRevision(), wire.Data.ScheduleRevision)
	require.NotContains(t, string(decoded.Payload), "entry_url", "durable intent must not copy the bearer entry URL")

	// The timer path shares the same persistence boundary and cannot bypass
	// staging by calling the legacy publisher after saving its task.
	scheduled := newTask(3)
	scheduler := appplan.NewTaskSchedulerServiceWithEnrollment(
		repository, nil, nil, runner, &uniqueOpenEntryGenerator{}, legacyPublisher,
		appEventing.ProfileBinding{Stager: stager},
	)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	openedTasks, err := scheduler.SchedulePendingTasks(t.Context(), 501, time.Now().Add(time.Second).In(shanghai).Format("2006-01-02 15:04:05"))
	require.NoError(t, err)
	require.Len(t, openedTasks, 1)
	require.Equal(t, scheduled.GetID().String(), openedTasks[0].ID)
	require.Empty(t, legacyPublisher.events)
	require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
	require.Equal(t, initialRows+2, rows)

	// Even when two callers load the same pending version before either saves,
	// only the CAS winner may stage one reminder identity.
	competingTask := newTask(4)
	barrier := &concurrentOpenRepository{
		AssessmentTaskRepository: repository,
		ready:                    make(chan struct{}, 2),
		release:                  make(chan struct{}),
	}
	concurrent := appplan.NewTaskManagementServiceWithEnrollment(
		barrier, nil, nil, runner, &uniqueOpenEntryGenerator{}, legacyPublisher,
		appEventing.ProfileBinding{Stager: stager},
	)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, openErr := concurrent.OpenTask(ctx, 501, competingTask.GetID().String())
			results <- openErr
		}()
	}
	for range 2 {
		select {
		case <-barrier.ready:
		case <-ctx.Done():
			t.Fatal("both callers did not load the pending task")
		}
	}
	close(barrier.release)
	var succeeded, conflicted int
	for range 2 {
		select {
		case openErr := <-results:
			switch {
			case openErr == nil:
				succeeded++
			case baseerrors.IsCode(openErr, code.ErrConflict):
				conflicted++
			default:
				t.Fatalf("unexpected competing opening result: %v", openErr)
			}
		case <-ctx.Done():
			t.Fatal("concurrent opening did not finish")
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)
	require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
	require.Equal(t, initialRows+3, rows)
	require.Empty(t, legacyPublisher.events)

	// The transaction can commit while its caller loses the success response.
	// A retry is rejected, so the caller must inspect the authoritative Task;
	// it cannot create another entry or reminder from the opened version.
	unknownTask := newTask(5)
	lostCommitResponse := errors.New("commit response lost after success")
	unknownRunner := apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if txErr := uow.WithinTransaction(ctx, fn); txErr != nil {
			return txErr
		}
		return lostCommitResponse
	})
	unknownService := appplan.NewTaskManagementServiceWithEnrollment(
		repository, nil, nil, unknownRunner, &uniqueOpenEntryGenerator{}, legacyPublisher,
		appEventing.ProfileBinding{Stager: stager},
	)
	_, err = unknownService.OpenTask(t.Context(), 501, unknownTask.GetID().String())
	require.ErrorIs(t, err, lostCommitResponse)
	committedTask, err := repository.FindByID(t.Context(), unknownTask.GetID())
	require.NoError(t, err)
	require.Equal(t, domainplan.TaskStatusOpened, committedTask.GetStatus())
	require.NotEmpty(t, committedTask.GetEntryURL())
	require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
	require.Equal(t, initialRows+4, rows)
	require.Empty(t, legacyPublisher.events)

	_, err = service.OpenTask(t.Context(), 501, unknownTask.GetID().String())
	require.Error(t, err, "an already-open Task must not be opened again")
	retriedTask, err := repository.FindByID(t.Context(), unknownTask.GetID())
	require.NoError(t, err)
	require.Equal(t, committedTask.GetEntryURL(), retriedTask.GetEntryURL())
	require.NoError(t, db.Table("rm_outbox").Count(&rows).Error)
	require.Equal(t, initialRows+4, rows)
	require.Empty(t, legacyPublisher.events)
}
