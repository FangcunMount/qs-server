// Package aibridge owns business request correlation and reliable delivery, not AI execution.
package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"
)

var ErrConflict = errors.New("AI bridge conflict")
var ErrInvalid = errors.New("invalid AI bridge message")
var ErrNotFound = errors.New("AI request not found")

type Actor struct {
	OrgID     string `json:"org_id"`
	SubjectID string `json:"subject_id"`
}
type Start struct {
	RequestID     string   `json:"request_id"`
	Actor         Actor    `json:"actor"`
	TesteeID      string   `json:"testee_id"`
	AssessmentIDs []string `json:"assessment_ids"`
	Goal          string   `json:"goal"`
}
type Change struct {
	CommandID       string  `json:"command_id"`
	SessionID       string  `json:"session_id"`
	Actor           Actor   `json:"actor"`
	Action          string  `json:"action"`
	ExpectedVersion int64   `json:"expected_version"`
	QuestionID      string  `json:"question_id"`
	Answer          *string `json:"answer,omitempty"`
	Skip            bool    `json:"skip"`
}
type Receipt struct {
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
}
type Event struct {
	EventID     string `json:"event_id"`
	RequestID   string `json:"request_id"`
	SessionID   string `json:"session_id"`
	Actor       Actor  `json:"actor"`
	TesteeID    string `json:"testee_id"`
	Version     int64  `json:"version"`
	Status      string `json:"status"`
	QuestionID  string `json:"question_id"`
	Question    string `json:"question"`
	CanSkip     bool   `json:"can_skip"`
	FailureCode string `json:"failure_code"`
}
type Command struct {
	ID        string
	RequestID string
	Kind      string
	Payload   json.RawMessage
}
type Store interface {
	StageStart(context.Context, Start) error
	StageChange(context.Context, string, Change) error
	Pending(context.Context, int) ([]Command, error)
	Acknowledge(context.Context, Command, Receipt) error
	Retry(context.Context, string) error
	Accept(context.Context, Event) error
	Projection(context.Context, string) (*Event, error)
}
type Sender interface {
	Send(context.Context, Command) (Receipt, error)
}
type Service struct {
	Store  Store
	Sender Sender
}

func validID(s string) bool { id, err := uuid.Parse(s); return err == nil && id.String() == s }
func validNumber(s string) bool {
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func (s *Service) Start(ctx context.Context, r Start) error {
	if !validID(r.RequestID) || !validNumber(r.Actor.OrgID) || r.Actor.SubjectID == "" || len(r.Actor.SubjectID) > 128 || !validNumber(r.TesteeID) || len(r.AssessmentIDs) == 0 || len(r.AssessmentIDs) > 10 || r.Goal == "" || len(r.Goal) > 8000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range r.AssessmentIDs {
		if !validNumber(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return s.Store.StageStart(ctx, r)
}
func (s *Service) Change(ctx context.Context, id string, r Change) error {
	if !validID(id) || !validID(r.CommandID) || !validID(r.SessionID) || r.ExpectedVersion < 1 || (r.Action != "answer" && r.Action != "cancel") {
		return ErrInvalid
	}
	if r.Action == "answer" && (!validID(r.QuestionID) || (r.Skip && r.Answer != nil) || (!r.Skip && (r.Answer == nil || *r.Answer == ""))) {
		return ErrInvalid
	}
	return s.Store.StageChange(ctx, id, r)
}
func (s *Service) Relay(ctx context.Context) (int, error) {
	commands, err := s.Store.Pending(ctx, 20)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, command := range commands {
		result, e := s.Sender.Send(ctx, command)
		if e == nil && (!validID(result.SessionID) || result.Version < 1) {
			e = ErrConflict
		}
		if e == nil {
			e = s.Store.Acknowledge(ctx, command, result)
		}
		if e != nil {
			if retryErr := s.Store.Retry(ctx, command.ID); retryErr != nil {
				return sent, retryErr
			}
			continue
		}
		sent++
	}
	return sent, nil
}
func (s *Service) Accept(ctx context.Context, event Event) error {
	if !validID(event.EventID) || !validID(event.RequestID) || !validID(event.SessionID) || event.Version < 1 {
		return ErrInvalid
	}
	switch event.Status {
	case "queued", "running", "awaiting_answer", "blocked", "cancelled":
	default:
		return ErrInvalid
	}
	if event.Status == "awaiting_answer" && (!validID(event.QuestionID) || event.Question == "") {
		return ErrInvalid
	}
	return s.Store.Accept(ctx, event)
}
