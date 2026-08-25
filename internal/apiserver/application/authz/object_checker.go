package authz

import (
	"context"
	"errors"
)

const (
	AssessmentResource        = "qs:evaluation:collection:assessments"
	ObjectOriginTypeAttribute = "object.origin_type"
)

var (
	ErrAuthorizationUnavailable = errors.New("authorization service unavailable")
	ErrAuthorizationContract    = errors.New("authorization contract or configuration error")
)

type ObjectCheckRequest struct {
	Subject    string
	Domain     string
	Resource   string
	Action     string
	ObjectID   string
	Attributes map[string]ObjectAttribute
}

type ObjectAttribute struct {
	String *string
	Int64  *int64
	Bool   *bool
}

func StringAttribute(value string) ObjectAttribute { return ObjectAttribute{String: &value} }

type ObjectDecision struct {
	Allowed              bool
	DenyCode             string
	PolicyVersion        int64
	MatchedGrantID       string
	MatchedRole          string
	MissingAttributeKeys []string
}

// ObjectAuthorizationChecker is the application port for authoritative IAM object checks.
type ObjectAuthorizationChecker interface {
	CheckObject(context.Context, ObjectCheckRequest) (ObjectDecision, error)
}
