package plan_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
	appplan "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	mysqlplan "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type concurrentOpenRepository struct {
	domainplan.AssessmentTaskRepository
	ready   chan struct{}
	release chan struct{}
}

func (r *concurrentOpenRepository) FindByID(ctx context.Context, id domainplan.AssessmentTaskID) (*domainplan.AssessmentTask, error) {
	task, err := r.AssessmentTaskRepository.FindByID(ctx, id)
	if err != nil || task == nil {
		return task, err
	}
	select {
	case r.ready <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-r.release:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type uniqueOpenEntryGenerator struct{ next atomic.Uint64 }

func (g *uniqueOpenEntryGenerator) GenerateEntry(context.Context, *domainplan.AssessmentTask) (string, string, error) {
	n := g.next.Add(1)
	return fmt.Sprintf("token-%d", n), fmt.Sprintf("https://example.test/entry/%d", n), nil
}

type openEventRecorder struct {
	mu     sync.Mutex
	events []event.DomainEvent
}

func (p *openEventRecorder) Publish(_ context.Context, evt event.DomainEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, evt)
	return nil
}

func (p *openEventRecorder) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func TestConcurrentTaskOpenHasOnePersistedEntryAndOneEventMySQL(t *testing.T) {
	dsn := os.Getenv("QS_M5_TASK_OPEN_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_M5_TASK_OPEN_MYSQL_REQUIRED") == "1" {
			t.Fatal("QS_M5_TASK_OPEN_MYSQL_DSN required")
		}
		t.Skip("disposable MySQL not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var database string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&database).Error)
	require.True(t, strings.HasPrefix(database, "qs_m5_task_open_test_"), "disposable database required")
	require.NoError(t, db.AutoMigrate(&mysqlplan.AssessmentTaskPO{}))

	repository := mysqlplan.NewTaskRepository(db)
	plannedAt := time.Now().Add(-time.Minute).UTC()
	task := domainplan.NewAssessmentTaskAt(domainplan.NewAssessmentPlanID(), 1, 7, testee.NewID(99), "scale", plannedAt, plannedAt)
	require.NoError(t, repository.Save(t.Context(), task))
	barrier := &concurrentOpenRepository{
		AssessmentTaskRepository: repository,
		ready:                    make(chan struct{}, 2),
		release:                  make(chan struct{}),
	}
	recorder := &openEventRecorder{}
	service := appplan.NewTaskManagementService(barrier, &uniqueOpenEntryGenerator{}, recorder)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, openErr := service.OpenTask(ctx, 7, task.GetID().String())
			results <- openErr
		}()
	}
	for range 2 {
		select {
		case <-barrier.ready:
		case <-ctx.Done():
			t.Fatal("both callers did not load the original pending task")
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
				t.Fatalf("unexpected opening result: %v", openErr)
			}
		case <-ctx.Done():
			t.Fatal("concurrent opening did not finish")
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)
	var persisted mysqlplan.AssessmentTaskPO
	require.NoError(t, db.Where("id = ?", task.GetID().Uint64()).Take(&persisted).Error)
	require.Equal(t, domainplan.TaskStatusOpened.String(), persisted.Status)
	require.NotEmpty(t, persisted.EntryToken)
	require.NotEmpty(t, persisted.EntryURL)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Len(t, recorder.events, 1, "the losing caller must not publish task.opened")
	require.Equal(t, domainplan.EventTypeTaskOpened, recorder.events[0].EventType())
	openedEvent, ok := recorder.events[0].(domainplan.TaskOpenedEvent)
	require.True(t, ok)
	require.Equal(t, persisted.EntryURL, openedEvent.Data.EntryURL, "published entry must match the single persisted winner")
}
