// Package operatorretirement models the durable exit of a backend operator.
package operatorretirement

import (
	"fmt"
	"strings"
	"time"
)

type Stage string

const (
	Disabled  Stage = "disabled"
	Revoked   Stage = "revoked"
	Completed Stage = "completed"
)

// Task records irreversible progress. Failure never reactivates an operator.
type Task struct {
	OperatorID      uint64
	OrgID           int64
	UserID          int64
	ActorID         int64
	RequestID       string
	Reason          string
	ExpectedVersion uint32
	Stage           Stage
	PolicyVersion   int64
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func New(operatorID uint64, orgID, userID, actorID int64, version uint32, requestID, reason string, now time.Time) (Task, error) {
	if operatorID == 0 || orgID <= 0 || userID <= 0 || actorID <= 0 || version == 0 || userID == actorID {
		return Task{}, fmt.Errorf("invalid operator retirement identity or version")
	}
	requestID, reason = strings.TrimSpace(requestID), strings.TrimSpace(reason)
	if requestID == "" || len(requestID) > 64 || reason == "" || len([]rune(reason)) > 500 {
		return Task{}, fmt.Errorf("retirement request ID and reason are required")
	}
	return Task{OperatorID: operatorID, OrgID: orgID, UserID: userID, ActorID: actorID, ExpectedVersion: version,
		RequestID: requestID, Reason: reason, Stage: Disabled, CreatedAt: now, UpdatedAt: now}, nil
}

func (t *Task) MarkRevoked(version int64, now time.Time) error {
	if t.Stage != Disabled || version <= 0 {
		return fmt.Errorf("invalid retirement revocation transition")
	}
	t.Stage, t.PolicyVersion, t.LastError, t.UpdatedAt = Revoked, version, "", now
	return nil
}
func (t *Task) Complete(observedVersion int64, hasRoles bool, now time.Time) error {
	if t.Stage != Revoked || t.PolicyVersion <= 0 || observedVersion < t.PolicyVersion || hasRoles {
		return fmt.Errorf("operator authorization has not converged")
	}
	t.Stage, t.LastError, t.UpdatedAt = Completed, "", now
	return nil
}
func (t Task) Validate() error {
	if t.OperatorID == 0 || t.OrgID <= 0 || t.UserID <= 0 || t.ActorID <= 0 || t.UserID == t.ActorID || t.ExpectedVersion == 0 || t.RequestID == "" || t.Reason == "" {
		return fmt.Errorf("invalid retirement task")
	}
	switch t.Stage {
	case Disabled:
		if t.PolicyVersion != 0 {
			return fmt.Errorf("invalid disabled task version")
		}
	case Revoked, Completed:
		if t.PolicyVersion <= 0 {
			return fmt.Errorf("missing committed policy version")
		}
	default:
		return fmt.Errorf("unknown retirement stage")
	}
	return nil
}
