package aibridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type RuntimeQuery struct {
	RequestID    string    `json:"request_id"`
	SessionID    string    `json:"session_id"`
	AssessmentID string    `json:"assessment_id"`
	TesteeID     string    `json:"testee_id"`
	Status       string    `json:"status"`
	History      bool      `json:"history"`
	Since        time.Time `json:"since"`
	Until        time.Time `json:"until"`
	Limit        int       `json:"limit"`
	Cursor       string    `json:"cursor"`
}

// Cursors bind filters and organization. They never confer authorization.
type RuntimeCursor struct {
	OrganizationID int64        `json:"org"`
	Query          RuntimeQuery `json:"query"`
	CreatedAt      *time.Time   `json:"created_at"`
	RequestID      string       `json:"request_id"`
}

func (q *RuntimeQuery) Normalize(now time.Time) error {
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 50 || len(q.Cursor) > 4096 {
		return ErrInvalid
	}
	if (q.RequestID != "" && !validID(q.RequestID)) || (q.SessionID != "" && !validID(q.SessionID)) || (q.AssessmentID != "" && !validNumber(q.AssessmentID)) || (q.TesteeID != "" && !validNumber(q.TesteeID)) {
		return ErrInvalid
	}
	switch q.Status {
	case "", "pending", "queued", "running", "awaiting_answer", "blocked", "cancelled", "completed":
	default:
		return ErrInvalid
	}
	if q.History && (!q.Since.IsZero() || !q.Until.IsZero()) {
		return ErrInvalid
	}
	if q.Cursor != "" {
		return nil
	} // original window is restored from the validated cursor
	exact := q.RequestID != "" || q.SessionID != "" || q.AssessmentID != "" || q.TesteeID != ""
	if !q.History && !exact {
		if q.Until.IsZero() {
			q.Until = now.UTC()
		}
		if q.Since.IsZero() {
			q.Since = q.Until.Add(-7 * 24 * time.Hour)
		}
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && !q.Since.Before(q.Until) {
		return ErrInvalid
	}
	return nil
}

func EncodeRuntimeCursor(c RuntimeCursor) string {
	c.Query.Cursor = ""
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func DecodeRuntimeCursor(raw string) (RuntimeCursor, error) {
	var c RuntimeCursor
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(raw) > 4096 || json.Unmarshal(data, &c) != nil || !validID(c.RequestID) || c.OrganizationID <= 0 || c.Query.Cursor != "" {
		return c, ErrInvalid
	}
	if err = c.Query.Normalize(time.Now()); err != nil {
		return c, err
	}
	return c, nil
}

type RuntimeRequest struct {
	RequestID       string          `json:"request_id"`
	SessionID       string          `json:"session_id"`
	TesteeID        string          `json:"testee_id"`
	AssessmentIDs   []string        `json:"assessment_ids"`
	Status          string          `json:"status"`
	Version         int64           `json:"version"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
	CommandsPending int             `json:"commands_pending"`
	CommandAttempts int             `json:"command_attempts"`
	AI              json.RawMessage `json:"ai,omitempty"`
}
type RuntimePage struct {
	Items          []RuntimeRequest `json:"items"`
	NextCursor     string           `json:"next_cursor"`
	ObservedAt     time.Time        `json:"observed_at"`
	Partial        bool             `json:"partial"`
	AIAvailability string           `json:"ai_availability"`
	AIObservedAt   string           `json:"ai_observed_at,omitempty"`
}
type RuntimeDetail struct {
	Request        RuntimeRequest  `json:"request"`
	ObservedAt     time.Time       `json:"observed_at"`
	Partial        bool            `json:"partial"`
	AIAvailability string          `json:"ai_availability"`
	AI             json.RawMessage `json:"ai,omitempty"`
}
type RuntimeStore interface {
	ListRuntime(context.Context, int64, RuntimeQuery) (RuntimePage, error)
	GetRuntime(context.Context, int64, string) (RuntimeRequest, error)
}
type RuntimeGateway interface {
	ReadRuntime(context.Context, DraftScope, []string, bool) (json.RawMessage, error)
}
type RuntimeAdministration struct {
	Store   RuntimeStore
	Gateway RuntimeGateway
}

func (s *RuntimeAdministration) authorize(ctx context.Context, scope DraftScope) error {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityOrgAdmin).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Store == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *RuntimeAdministration) List(ctx context.Context, scope DraftScope, q RuntimeQuery) (RuntimePage, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return RuntimePage{}, err
	}
	if err := q.Normalize(time.Now()); err != nil {
		return RuntimePage{}, err
	}
	page, err := s.Store.ListRuntime(ctx, scope.OrganizationID, q)
	if err != nil {
		return page, err
	}
	ids := []string{}
	for _, item := range page.Items {
		if item.SessionID != "" {
			ids = append(ids, item.SessionID)
		}
	}
	page.AIAvailability = "not_requested"
	if len(ids) == 0 {
		return page, nil
	}
	page.AIAvailability = "unavailable"
	page.Partial = true
	if s.Gateway == nil {
		return page, nil
	}
	raw, err := s.Gateway.ReadRuntime(ctx, scope, ids, false)
	if errors.Is(err, ErrGovernanceDenied) {
		return RuntimePage{}, err
	}
	if err != nil {
		return page, nil
	}
	var batch struct {
		Items       []json.RawMessage `json:"items"`
		ObservedAt  string            `json:"observed_at"`
		Unavailable []string          `json:"unavailable_session_ids"`
	}
	if json.Unmarshal(raw, &batch) != nil {
		return page, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, batch.ObservedAt); err != nil {
		return page, nil
	}
	expected := map[string]bool{}
	for _, item := range page.Items {
		if item.SessionID != "" {
			expected[item.SessionID+":"+item.RequestID] = true
		}
	}
	found := map[string]json.RawMessage{}
	for _, item := range batch.Items {
		var v struct {
			SessionID string `json:"session_id"`
			RequestID string `json:"request_id"`
		}
		if json.Unmarshal(item, &v) != nil {
			return page, nil
		}
		key := v.SessionID + ":" + v.RequestID
		if !expected[key] || found[key] != nil {
			return page, nil
		}
		found[key] = item
	}
	page.Partial = false
	page.AIAvailability = "available"
	page.AIObservedAt = batch.ObservedAt
	for i := range page.Items {
		item := &page.Items[i]
		if item.SessionID != "" {
			item.AI = found[item.SessionID+":"+item.RequestID]
			if item.AI == nil {
				page.Partial = true
			}
		}
	}
	if page.Partial {
		page.AIAvailability = "partial"
	}
	return page, nil
}
func (s *RuntimeAdministration) Get(ctx context.Context, scope DraftScope, id string) (RuntimeDetail, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return RuntimeDetail{}, err
	}
	if !validID(id) {
		return RuntimeDetail{}, ErrInvalid
	}
	item, err := s.Store.GetRuntime(ctx, scope.OrganizationID, id)
	if err != nil {
		return RuntimeDetail{}, err
	}
	result := RuntimeDetail{Request: item, ObservedAt: time.Now().UTC(), AIAvailability: "not_requested"}
	if item.SessionID == "" {
		return result, nil
	}
	result.Partial = true
	result.AIAvailability = "unavailable"
	if s.Gateway == nil {
		return result, nil
	}
	raw, err := s.Gateway.ReadRuntime(ctx, scope, []string{item.SessionID}, true)
	if errors.Is(err, ErrGovernanceDenied) {
		return RuntimeDetail{}, err
	}
	if err != nil {
		return result, nil
	}
	var v struct {
		Execution struct {
			SessionID string `json:"session_id"`
			RequestID string `json:"request_id"`
		} `json:"execution"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Execution.SessionID != item.SessionID || v.Execution.RequestID != id {
		return result, nil
	}
	result.AI = raw
	result.Partial = false
	result.AIAvailability = "available"
	return result, nil
}

type RuntimeHealthStore interface {
	RuntimeBacklog(context.Context, int64) (map[string]int64, error)
}

type RuntimeHealthGateway interface {
	ReadRuntimeHealth(context.Context, DraftScope) (json.RawMessage, error)
}

func (s *RuntimeAdministration) Health(ctx context.Context, scope DraftScope) (map[string]any, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return nil, err
	}
	result := map[string]any{"observed_at": time.Now().UTC(), "partial": true, "ai_availability": "unavailable"}
	if store, ok := s.Store.(RuntimeHealthStore); ok {
		counts, err := store.RuntimeBacklog(ctx, scope.OrganizationID)
		if err != nil {
			return nil, err
		}
		result["qs"] = counts
	}
	if gateway, ok := s.Gateway.(RuntimeHealthGateway); ok {
		raw, err := gateway.ReadRuntimeHealth(ctx, scope)
		if errors.Is(err, ErrGovernanceDenied) {
			return nil, err
		}
		if err == nil {
			result["ai"] = raw
			result["ai_availability"] = "available"
			result["partial"] = false
		}
	}
	return result, nil
}

type RuntimeTimelineEvent struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Kind         string     `json:"kind"`
	At           *time.Time `json:"at"`
	RunID        string     `json:"run_id,omitempty"`
	InvocationID string     `json:"invocation_id,omitempty"`
	Version      int64      `json:"version,omitempty"`
	Attempt      int        `json:"attempt,omitempty"`
}

func (s *RuntimeAdministration) Timeline(ctx context.Context, scope DraftScope, id string) (map[string]any, error) {
	detail, err := s.Get(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	events := []RuntimeTimelineEvent{{ID: "qs:accepted:" + id, Source: "qs", Kind: "request_accepted", At: detail.Request.CreatedAt}}
	if detail.Request.Version > 0 {
		events = append(events, RuntimeTimelineEvent{ID: "qs:projection:" + id, Source: "qs", Kind: "result_received", At: detail.Request.UpdatedAt, Version: detail.Request.Version})
	}
	var evidence struct {
		Milestones []struct {
			Key          string     `json:"dedupe_key"`
			Kind         string     `json:"kind"`
			RunID        string     `json:"run_id"`
			InvocationID string     `json:"invocation_id"`
			Attempt      int        `json:"attempt"`
			At           *time.Time `json:"occurred_at"`
		} `json:"milestones"`
		Deliveries []struct {
			ID        string     `json:"event_id"`
			Version   int64      `json:"version"`
			Created   *time.Time `json:"created_at"`
			Delivered *time.Time `json:"delivered_at"`
		} `json:"deliveries"`
	}
	if len(detail.AI) > 0 && json.Unmarshal(detail.AI, &evidence) == nil {
		for _, v := range evidence.Milestones {
			events = append(events, RuntimeTimelineEvent{ID: "ai:" + v.Key, Source: "ai", Kind: v.Kind, At: v.At, RunID: v.RunID, InvocationID: v.InvocationID, Attempt: v.Attempt})
		}
		for _, v := range evidence.Deliveries {
			events = append(events, RuntimeTimelineEvent{ID: "ai:outbox:" + v.ID, Source: "ai", Kind: "result_staged", At: v.Created, Version: v.Version})
			if v.Delivered != nil {
				events = append(events, RuntimeTimelineEvent{ID: "ai:delivered:" + v.ID, Source: "ai", Kind: "delivery_confirmed", At: v.Delivered, Version: v.Version})
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At == nil {
			return events[j].At != nil
		}
		if events[j].At == nil {
			return false
		}
		return events[i].At.Before(*events[j].At)
	})
	return map[string]any{"request_id": id, "observed_at": detail.ObservedAt, "partial": detail.Partial, "history_complete": false, "events": events}, nil
}
