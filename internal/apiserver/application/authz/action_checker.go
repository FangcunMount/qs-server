package authz

import (
	"context"
	"errors"
)

var (
	ErrAuthorizationUnavailable = errors.New("authorization service unavailable")
	ErrAuthorizationContract    = errors.New("authorization contract or configuration error")
)

type ActionCheckRequest struct {
	Subject string

	Resource string
	Action   string
}

type ActionDecision struct {
	Allowed        bool
	DenyCode       string
	PolicyVersion  int64
	MatchedGrantID string
	MatchedRole    string
}

// ActionAuthorizationChecker is the application port for authoritative IAM action checks.
type ActionAuthorizationChecker interface {
	CheckAction(context.Context, ActionCheckRequest) (ActionDecision, error)
}
