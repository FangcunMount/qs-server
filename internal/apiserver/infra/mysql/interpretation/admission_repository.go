package interpretation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

// AdmissionFailurePO is the relational form of lifecycle-front admission evidence.
type AdmissionFailurePO struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement:false"`
	OutcomeID      uint64    `gorm:"column:outcome_id;not null"`
	OrgID          int64     `gorm:"column:org_id;not null"`
	AssessmentID   uint64    `gorm:"column:assessment_id;not null"`
	TesteeID       uint64    `gorm:"column:testee_id;not null"`
	EventID        string    `gorm:"column:event_id;size:128;not null"`
	TraceID        string    `gorm:"column:trace_id;size:128;not null"`
	Kind           string    `gorm:"column:kind;size:64;not null"`
	Code           string    `gorm:"column:code;size:128;not null"`
	SafeMessage    string    `gorm:"column:safe_message;type:text;not null"`
	Retryable      bool      `gorm:"column:retryable;not null"`
	Fingerprint    string    `gorm:"column:fingerprint;size:191;not null"`
	GenerationID   uint64    `gorm:"column:generation_id;not null"`
	OutcomeVersion string    `gorm:"column:outcome_version;size:128;not null"`
	Attempt        uint      `gorm:"column:attempt;not null"`
	Decision       string    `gorm:"column:decision;size:32;not null"`
	FirstFailedAt  time.Time `gorm:"column:first_failed_at;not null"`
	LastFailedAt   time.Time `gorm:"column:last_failed_at;not null"`
	OccurredAt     time.Time `gorm:"column:occurred_at;not null"`
}

func (AdmissionFailurePO) TableName() string { return "interpretation_admission_failure" }

// AdmissionFailureRepository persists admission evidence in MySQL. It remains
// outside the report-generation transaction because admission failures happen
// before Generation/Run creation.
type AdmissionFailureRepository struct {
	db      *gorm.DB
	limiter backpressure.Acquirer
}

func NewAdmissionFailureRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) *AdmissionFailureRepository {
	options := mysql.BaseRepositoryOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	return &AdmissionFailureRepository{db: db, limiter: options.Limiter}
}

var _ admission.QueryRepository = (*AdmissionFailureRepository)(nil)

func (r *AdmissionFailureRepository) acquire(ctx context.Context) (context.Context, func(), error) {
	if r == nil || r.limiter == nil {
		return ctx, func() {}, nil
	}
	return r.limiter.Acquire(ctx)
}

func (r *AdmissionFailureRepository) UpsertByFingerprint(ctx context.Context, failure *admission.Failure) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("interpretation admission failure repository is not configured")
	}
	if failure == nil {
		return false, fmt.Errorf("admission failure is required")
	}
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	po := admissionFailureToPO(failure)
	result := r.db.WithContext(ctx).Create(po)
	if result.Error == nil {
		return true, nil
	}
	if !mysql.IsDuplicateError(result.Error) {
		return false, fmt.Errorf("insert interpretation admission failure: %w", result.Error)
	}

	result = r.db.WithContext(ctx).Model(&AdmissionFailurePO{}).
		Where("fingerprint = ?", po.Fingerprint).
		Updates(map[string]any{
			"attempt":        gorm.Expr("attempt + 1"),
			"last_failed_at": po.LastFailedAt,
			"trace_id":       po.TraceID,
		})
	if result.Error != nil {
		return false, fmt.Errorf("update interpretation admission failure: %w", result.Error)
	}
	// A collision on the immutable domain id but not on fingerprint preserves
	// the idempotency contract: no new evidence is created or mutated.
	return false, nil
}

func (r *AdmissionFailureRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*admission.Failure, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("interpretation admission failure repository is not configured")
	}
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var po AdmissionFailurePO
	if err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&po).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, admission.ErrNotFound
		}
		return nil, fmt.Errorf("find admission failure by fingerprint: %w", err)
	}
	return admissionFailureToDomain(&po)
}

func (r *AdmissionFailureRepository) FindByOutcomeID(ctx context.Context, outcomeID meta.ID, limit int) ([]*admission.Failure, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("interpretation admission failure repository is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	var rows []AdmissionFailurePO
	if err := r.db.WithContext(ctx).Where("outcome_id = ?", outcomeID.Uint64()).
		Order("occurred_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list admission failures: %w", err)
	}
	return admissionFailuresToDomain(rows)
}

func (r *AdmissionFailureRepository) ListFailures(ctx context.Context, filter admission.QueryFilter, cursor string, limit int) (admission.QueryPage, error) {
	if r == nil || r.db == nil {
		return admission.QueryPage{}, fmt.Errorf("interpretation admission failure repository is not configured")
	}
	if filter.OrgID == 0 {
		return admission.QueryPage{}, fmt.Errorf("admission failure query requires organization")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return admission.QueryPage{}, err
	}
	defer release()
	query := r.db.WithContext(ctx).Model(&AdmissionFailurePO{}).Where("org_id = ?", filter.OrgID)
	if filter.Kind != nil {
		query = query.Where("kind = ?", string(*filter.Kind))
	}
	if filter.Decision != "" {
		query = query.Where("decision = ?", filter.Decision)
	}
	if filter.AssessmentID != nil {
		query = query.Where("assessment_id = ?", filter.AssessmentID.Uint64())
	}
	if filter.OutcomeID != nil {
		query = query.Where("outcome_id = ?", filter.OutcomeID.Uint64())
	}
	if filter.OccurredFrom != nil {
		query = query.Where("occurred_at >= ?", filter.OccurredFrom.UTC())
	}
	if filter.OccurredTo != nil {
		query = query.Where("occurred_at <= ?", filter.OccurredTo.UTC())
	}
	cursorAt, cursorID, err := decodeAdmissionCursor(cursor)
	if err != nil {
		return admission.QueryPage{}, err
	}
	if !cursorAt.IsZero() {
		query = query.Where("(occurred_at < ?) OR (occurred_at = ? AND id < ?)", cursorAt, cursorAt, cursorID)
	}
	var rows []AdmissionFailurePO
	if err := query.Order("occurred_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return admission.QueryPage{}, fmt.Errorf("query admission failures: %w", err)
	}
	items, err := admissionFailuresToDomain(rows)
	if err != nil {
		return admission.QueryPage{}, err
	}
	page := admission.QueryPage{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = encodeAdmissionCursor(last.OccurredAt(), last.ID().Uint64())
	}
	return page, nil
}

func admissionFailureToPO(failure *admission.Failure) *AdmissionFailurePO {
	return &AdmissionFailurePO{
		ID: failure.ID().Uint64(), OutcomeID: failure.OutcomeID().Uint64(), OrgID: failure.OrgID(),
		AssessmentID: failure.AssessmentID().Uint64(), TesteeID: failure.TesteeID(), EventID: failure.EventID(), TraceID: failure.TraceID(),
		Kind: string(failure.Kind()), Code: failure.Code(), SafeMessage: failure.SafeMessage(), Retryable: failure.Retryable(),
		Fingerprint: failure.Fingerprint(), GenerationID: failure.GenerationID().Uint64(), OutcomeVersion: failure.OutcomeVersion(),
		Attempt: failure.Attempt(), Decision: failure.Decision(), FirstFailedAt: failure.FirstFailedAt(),
		LastFailedAt: failure.LastFailedAt(), OccurredAt: failure.OccurredAt(),
	}
}

func admissionFailureToDomain(po *AdmissionFailurePO) (*admission.Failure, error) {
	if po == nil {
		return nil, fmt.Errorf("admission failure persistence object is required")
	}
	return admission.NewFailure(admission.Input{
		ID: meta.FromUint64(po.ID), OutcomeID: meta.FromUint64(po.OutcomeID), OrgID: po.OrgID,
		AssessmentID: meta.FromUint64(po.AssessmentID), TesteeID: po.TesteeID, EventID: po.EventID, TraceID: po.TraceID,
		Kind: admission.Kind(po.Kind), Code: po.Code, SafeMessage: po.SafeMessage, Retryable: po.Retryable,
		GenerationID: meta.FromUint64(po.GenerationID), OutcomeVersion: po.OutcomeVersion, Attempt: po.Attempt,
		Decision: po.Decision, FirstFailedAt: po.FirstFailedAt, LastFailedAt: po.LastFailedAt, OccurredAt: po.OccurredAt,
	})
}

func admissionFailuresToDomain(rows []AdmissionFailurePO) ([]*admission.Failure, error) {
	items := make([]*admission.Failure, 0, len(rows))
	for i := range rows {
		item, err := admissionFailureToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func encodeAdmissionCursor(at time.Time, id uint64) string {
	raw := strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAdmissionCursor(value string) (time.Time, uint64, error) {
	if value == "" {
		return time.Time{}, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || id == 0 {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	return time.Unix(0, nanos).UTC(), id, nil
}
