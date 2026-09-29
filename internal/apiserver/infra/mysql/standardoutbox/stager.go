//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"gorm.io/gorm"
)

// Stager writes standard messages only through the host's existing GORM
// transaction, including scheduled events from the same business boundary.
type Stager struct {
	resolver eventcatalog.TopicResolver
	source   string
}

func NewStager(resolver eventcatalog.TopicResolver, source string) (*Stager, error) {
	if resolver == nil || source == "" {
		return nil, fmt.Errorf("standard MySQL stager requires topic resolver and source")
	}
	return &Stager{resolver: resolver, source: source}, nil
}

func (s *Stager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return s.stageAt(ctx, time.Time{}, events)
}

func (s *Stager) StageAt(ctx context.Context, dueAt time.Time, events ...event.DomainEvent) error {
	if dueAt.IsZero() {
		return fmt.Errorf("scheduled standard message requires due time")
	}
	return s.stageAt(ctx, dueAt, events)
}

func (s *Stager) stageAt(ctx context.Context, dueAt time.Time, events []event.DomainEvent) error {
	if s == nil || s.resolver == nil || s.source == "" {
		return fmt.Errorf("standard MySQL stager is not configured")
	}
	if len(events) == 0 {
		return nil
	}
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	appender, err := sdkmysql.BindGORM(tx)
	if err != nil {
		return err
	}
	prepared, err := standardoutbox.PrepareIntents(events, s.resolver, s.source)
	if err != nil {
		return err
	}
	for index, item := range prepared {
		when := item.DueAt
		if !dueAt.IsZero() {
			when = dueAt
		}
		if err := appender.Append(ctx, item.Message, when); err != nil {
			return err
		}
		if item.Message.Input().EventType == eventcatalog.EvaluationRequested {
			if err := stageEvaluationRequestRef(ctx, tx, events[index], item.Message.Input().ID, item.Message.Input().Scope); err != nil {
				return err
			}
		}
	}
	return nil
}

// stageEvaluationRequestRef preserves the original event identity in the
// same business transaction as the Assessment and SDK Outbox append. A later
// recovery may look up candidates by Assessment ID without searching payloads.
func stageEvaluationRequestRef(ctx context.Context, tx *gorm.DB, evt event.DomainEvent, eventID, scope string) error {
	if evt.AggregateType() != "Evaluation" || evt.EventID() != eventID || !strings.HasPrefix(scope, "org:") {
		return fmt.Errorf("invalid evaluation request identity")
	}
	assessmentID, err := strconv.ParseUint(evt.AggregateID(), 10, 64)
	if err != nil || assessmentID == 0 {
		return fmt.Errorf("invalid evaluation request assessment ID")
	}
	orgID, err := strconv.ParseInt(strings.TrimPrefix(scope, "org:"), 10, 64)
	if err != nil || orgID <= 0 {
		return fmt.Errorf("invalid evaluation request organization scope")
	}
	insert := tx.WithContext(ctx).Exec(`INSERT INTO qs_rm_evaluation_request_ref
	(event_id,assessment_id,org_id)
	SELECT ?,id,org_id FROM assessment
	WHERE id=? AND org_id=? AND status='submitted' AND deleted_at IS NULL`, eventID, assessmentID, orgID)
	if err := insert.Error; err != nil {
		return fmt.Errorf("stage evaluation request identity: %w", err)
	}
	if insert.RowsAffected != 1 {
		return fmt.Errorf("evaluation request assessment identity is not submitted in its organization")
	}
	return nil
}
