package systemgovernance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	redis "github.com/redis/go-redis/v9"
)

var auditFallbackPending = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "system_governance_audit_fallback_pending",
	Help: "Current governance audit terminal outcomes awaiting MySQL recovery.",
})

var auditFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "system_governance_audit_fallback_total",
	Help: "Total governance audit fallback operations.",
}, []string{"operation", "outcome"})

type ActionAuditFallbackStore struct {
	client  redis.UniversalClient
	builder *keyspace.Builder
}

type actionAuditFallbackValue struct {
	SchemaVersion int                   `json:"schema_version"`
	OrgID         int64                 `json:"org_id"`
	RequestID     string                `json:"request_id"`
	ActionID      string                `json:"action_id"`
	Status        string                `json:"status"`
	FinishedAt    time.Time             `json:"finished_at"`
	Result        *app.ActionRunResult  `json:"result,omitempty"`
	Error         *app.ActionAuditError `json:"error,omitempty"`
}

func NewActionAuditFallbackStore(client redis.UniversalClient, builder *keyspace.Builder) *ActionAuditFallbackStore {
	if builder == nil {
		builder = keyspace.NewBuilder()
	}
	return &ActionAuditFallbackStore{client: client, builder: builder}
}

func (s *ActionAuditFallbackStore) Load(ctx context.Context, orgID int64, requestID string) (app.ActionAuditRecord, bool, error) {
	if s == nil || s.client == nil {
		return app.ActionAuditRecord{}, false, errors.New("governance audit fallback redis is unavailable")
	}
	raw, err := s.client.Get(ctx, s.key(orgID, requestID)).Bytes()
	if errors.Is(err, redis.Nil) {
		auditFallbackTotal.WithLabelValues("load", "miss").Inc()
		return app.ActionAuditRecord{}, false, nil
	}
	if err != nil {
		auditFallbackTotal.WithLabelValues("load", "failed").Inc()
		return app.ActionAuditRecord{}, false, err
	}
	record, err := decodeActionAuditFallback(raw)
	if err != nil {
		auditFallbackTotal.WithLabelValues("load", "failed").Inc()
		return app.ActionAuditRecord{}, false, err
	}
	auditFallbackTotal.WithLabelValues("load", "ok").Inc()
	return record, true, nil
}

func (s *ActionAuditFallbackStore) Put(ctx context.Context, record app.ActionAuditRecord) error {
	if s == nil || s.client == nil {
		return errors.New("governance audit fallback redis is unavailable")
	}
	value := actionAuditFallbackValue{
		SchemaVersion: 1, OrgID: record.OrgID, RequestID: record.RequestID,
		ActionID: record.ActionID, Status: record.Status, FinishedAt: record.FinishedAt,
		Result: record.Result, Error: record.Error,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	key := s.key(record.OrgID, record.RequestID)
	created, err := s.client.SetNX(ctx, key, raw, 0).Result()
	if err != nil {
		auditFallbackTotal.WithLabelValues("put", "failed").Inc()
		return err
	}
	if !created {
		existing, loadErr := s.client.Get(ctx, key).Bytes()
		if loadErr != nil {
			return loadErr
		}
		if !bytes.Equal(existing, raw) {
			return errors.New("governance audit fallback terminal conflicts with existing record")
		}
		auditFallbackTotal.WithLabelValues("put", "noop").Inc()
		return nil
	}
	auditFallbackPending.Inc()
	auditFallbackTotal.WithLabelValues("put", "ok").Inc()
	return nil
}

func (s *ActionAuditFallbackStore) Delete(ctx context.Context, orgID int64, requestID string) error {
	if s == nil || s.client == nil {
		return errors.New("governance audit fallback redis is unavailable")
	}
	deleted, err := s.client.Del(ctx, s.key(orgID, requestID)).Result()
	if err != nil {
		auditFallbackTotal.WithLabelValues("delete", "failed").Inc()
		return err
	}
	if deleted > 0 {
		auditFallbackPending.Dec()
	}
	auditFallbackTotal.WithLabelValues("delete", "ok").Inc()
	return nil
}

func (s *ActionAuditFallbackStore) List(ctx context.Context, limit int) ([]app.ActionAuditRecord, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("governance audit fallback redis is unavailable")
	}
	if limit <= 0 {
		limit = 100
	}
	pattern := s.builder.BuildGovernanceAuditReplayKey("*", "*")
	records := make([]app.ActionAuditRecord, 0, limit)
	pending := 0
	iter := s.client.Scan(ctx, 0, pattern, int64(limit)).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Bytes()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		record, err := decodeActionAuditFallback(raw)
		if err != nil {
			return nil, fmt.Errorf("decode governance audit fallback %q: %w", iter.Val(), err)
		}
		pending++
		if len(records) < limit {
			records = append(records, record)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	auditFallbackPending.Set(float64(pending))
	auditFallbackTotal.WithLabelValues("list", "ok").Inc()
	return records, nil
}

func (s *ActionAuditFallbackStore) key(orgID int64, requestID string) string {
	return s.builder.BuildGovernanceAuditReplayKey(strconv.FormatInt(orgID, 10), requestID)
}

func decodeActionAuditFallback(raw []byte) (app.ActionAuditRecord, error) {
	var value actionAuditFallbackValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return app.ActionAuditRecord{}, err
	}
	if value.SchemaVersion != 1 || value.OrgID == 0 || value.RequestID == "" || value.ActionID == "" || value.Status == "" || value.FinishedAt.IsZero() {
		return app.ActionAuditRecord{}, errors.New("invalid governance audit fallback value")
	}
	return app.ActionAuditRecord{
		OrgID: value.OrgID, RequestID: value.RequestID, ActionID: value.ActionID,
		Status: value.Status, FinishedAt: value.FinishedAt, Result: value.Result, Error: value.Error,
	}, nil
}

var _ app.ActionAuditFallbackStore = (*ActionAuditFallbackStore)(nil)
