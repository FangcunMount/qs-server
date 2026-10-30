package taskopen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	notification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

var ErrUnsafeRecoveryOpening = errors.New("original committed task opening is unavailable or differs from current task")

// RecoveryOpening is private recovery authority, never a public response. The
// full artifact and entry URL must not leave the host or reach a new MQ send.
type RecoveryOpening struct {
	Task        repository.AssessmentTaskPO            `json:"-"`
	Request     notification.TaskOpenedReminderRequest `json:"-"`
	Reference   []byte                                 `json:"-"`
	Fingerprint string                                 `json:"-"`
}

func (o RecoveryOpening) MatchesCurrent(task *repository.AssessmentTaskPO) bool {
	if task == nil || task.ID != o.Task.ID || task.OrgID != o.Task.OrgID || task.PlanID != o.Task.PlanID || task.TesteeID != o.Task.TesteeID || task.ScheduleRevision != o.Task.ScheduleRevision || task.Status != domain.TaskStatusOpened.String() || !sameOptionalClock(task.OpenAt, o.Task.OpenAt) || !sameOptionalClock(task.ExpireAt, o.Task.ExpireAt) || task.EntryURL != o.Task.EntryURL {
		return false
	}
	// Match the same stable input as the production opening recovery. A silent
	// edit without a revision bump must not authorize a different task schedule.
	current, err := (frozenOpening{Task: *task}).inputHash()
	original, originalErr := (frozenOpening{Task: o.Task}).inputHash()
	return err == nil && originalErr == nil && current == original
}

// readRecoveryOpening reuses the production opening artifact and consumer-wire
// checks, without constructing a sender or granting an execution capability.
func readRecoveryOpening(plan transaction.FrozenPlan) (RecoveryOpening, error) {
	raw, err := boundary.Artifact(plan)
	if err != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	f, err := read(raw)
	if err != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	in := plan.Input()
	hash, err := f.inputHash()
	if err != nil || in.Owner != "qs.mysql" || in.Scope != fmt.Sprintf("org:%d", f.Task.OrgID) || in.IdempotencyKey != openingKey(f.Task.ID.Uint64(), f.Task.ScheduleRevision) || in.InputHash != hash || len(in.Branches) != 1 {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	b := in.Branches[0]
	m := b.Message.Input()
	m.Payload = bytes.Clone(b.Wire)
	wire, err := message.New(m)
	if err != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	ref := standard.ReferenceFromMessage(wire)
	env, err := standard.VerifyReference(m, ref.Fingerprint, ref)
	if err != nil || m.Destination != "qs.plan.task" || m.EventType != domain.EventTypeTaskOpenedReminderRequested || env.AggregateType != domain.AggregateTypeTask || env.AggregateID != f.Task.ID.String() || !bytes.Equal(env.Data, f.Reminder) {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	var intent struct {
		OrgID            int64     `json:"org_id"`
		TaskID           string    `json:"task_id"`
		TesteeID         string    `json:"testee_id"`
		ScheduleRevision uint32    `json:"schedule_revision"`
		OpenAt           time.Time `json:"open_at"`
	}
	if json.Unmarshal(env.Data, &intent) != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	return RecoveryOpening{Task: f.Task, Request: notification.TaskOpenedReminderRequest{OpeningEventID: m.ID, Intent: notification.TaskOpenedReminderIntent{OrgID: intent.OrgID, TaskID: intent.TaskID, TesteeID: intent.TesteeID, ScheduleRevision: intent.ScheduleRevision, OpenAt: intent.OpenAt}}}, nil
}

type RecoverySource struct {
	db  *gorm.DB
	now func() time.Time
}

func NewRecoverySource(db *gorm.DB) *RecoverySource { return &RecoverySource{db: db, now: time.Now} }

// Capture owns a read-only original-primary snapshot. Only exact committed
// evidence is accepted; local acceptance is not Broker confirmation.
func (s *RecoverySource) Capture(ctx context.Context, orgID int64, taskID uint64, revision uint32, openedBefore time.Time) (RecoveryOpening, error) {
	if s == nil || s.db == nil || orgID <= 0 || taskID == 0 || revision == 0 || openedBefore.IsZero() {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	tx := s.db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return RecoveryOpening{}, tx.Error
	}
	defer func() { _ = tx.Rollback().Error }()
	q := transaction.Request{Owner: "qs.mysql", Scope: fmt.Sprintf("org:%d", orgID), Key: openingKey(taskID, revision)}
	key := q.KeyDigest()
	var request struct {
		Owner, Scope, IdempotencyKey string
		InputHash, ActiveExecution   []byte
	}
	if err := tx.Raw("SELECT owner,scope,idempotency_key,input_hash,active_execution FROM rm_tx_request WHERE request_key=?", key[:]).Take(&request).Error; err != nil {
		return RecoveryOpening{}, err
	}
	if request.Owner != q.Owner || request.Scope != q.Scope || request.IdempotencyKey != q.Key || len(request.InputHash) != 32 || len(request.ActiveExecution) != 32 {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	copy(q.InputHash[:], request.InputHash)
	var row struct {
		Owner       string
		RequestKey  []byte
		OperationID string
		Plan        []byte
		Fence       uint64
		WriterEpoch string
		State       transaction.State
	}
	if err := tx.Raw("SELECT owner,request_key,operation_id,plan,fence,writer_epoch,state FROM rm_tx_execution WHERE execution_key=?", request.ActiveExecution).Take(&row).Error; err != nil {
		return RecoveryOpening{}, err
	}
	plan, err := transaction.DecodePlan(row.Plan)
	if err != nil || row.Owner != q.Owner || !bytes.Equal(row.RequestKey, key[:]) || row.OperationID != plan.Input().OperationID || plan.MatchRequest(q.Owner, q.Scope, q.Key, q.InputHash) != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	o, err := readRecoveryOpening(plan)
	if err != nil || o.Task.ID.Uint64() != taskID || o.Task.OrgID != orgID || o.Task.ScheduleRevision != revision {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	ref, err := plan.Reference(plan.Input().Branches[0].ID, row.Fence, row.WriterEpoch)
	if err != nil {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	ek, bk := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence), transaction.BranchKey(ref.BranchID)
	if !bytes.Equal(ek[:], request.ActiveExecution) {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	var witness struct {
		Reference []byte
		State     transaction.State
	}
	if err := tx.Raw("SELECT reference,state FROM rm_tx_witness WHERE execution_key=? AND branch_key=?", ek[:], bk[:]).Take(&witness).Error; err != nil {
		return RecoveryOpening{}, err
	}
	o.Reference, err = transaction.EncodeReference(ref)
	if err != nil || !bytes.Equal(witness.Reference, o.Reference) || transaction.Resolve(ref, &transaction.Evidence{Reference: ref, ExecutionState: row.State, BranchState: witness.State}) != transaction.Commit {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	var binding struct {
		StoreID       string
		SchemaVersion int
	}
	if err := tx.Raw("SELECT store_id,schema_version FROM rm_tx_binding WHERE owner=?", []byte(q.Owner)).Take(&binding).Error; err != nil {
		return RecoveryOpening{}, err
	}
	if binding.StoreID == "" || binding.SchemaVersion != 1 {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	var current repository.AssessmentTaskPO
	if err := tx.Table("assessment_task").Where("id=? AND org_id=? AND deleted_at IS NULL", taskID, orgID).Take(&current).Error; err != nil {
		return RecoveryOpening{}, err
	}
	if !o.MatchesCurrent(&current) || current.OpenAt == nil || current.OpenAt.After(openedBefore) {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	decision, err := notification.EvaluateTaskOpenedReminder(o.Request.Intent, &planapp.TaskReminderState{OrgID: orgID, TaskID: o.Request.Intent.TaskID, TesteeID: o.Request.Intent.TesteeID, Status: domain.TaskStatus(current.Status), ScheduleRevision: current.ScheduleRevision, OpenAt: current.OpenAt, ExpireAt: current.ExpireAt, EntryURL: current.EntryURL}, s.now())
	if err != nil || decision.SuppressCode != "" {
		return RecoveryOpening{}, ErrUnsafeRecoveryOpening
	}
	digest := plan.Digest()
	input, err := json.Marshal(struct {
		Reference []byte
		Plan      [32]byte
		Store     string
	}{o.Reference, digest, binding.StoreID})
	if err != nil {
		return RecoveryOpening{}, err
	}
	fingerprint := sha256.Sum256(input)
	o.Fingerprint = hex.EncodeToString(fingerprint[:])
	return o, nil
}
