package planentry

import (
	"context"
	"errors"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
)

var ErrInvalidEntry = errors.New("plan task entry not found")

type Entry struct {
	TaskID               string `json:"task_id"`
	TesteeID             string `json:"testee_id"`
	ScaleCode            string `json:"scale_code"`
	ExpiresAt            string `json:"expires_at"`
	PlanID               string `json:"plan_id"`
	Title                string `json:"title"`
	OpenAt               string `json:"open_at"`
	DueAt                string `json:"due_at"`
	Status               string `json:"status"`
	CanStart             bool   `json:"can_start"`
	QuestionnaireCode    string `json:"q"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	ModelVersion         string `json:"model_version"`
}

type Reader interface {
	ResolveTaskEntry(context.Context, string, string) (*Entry, error)
}

type AccessAuthorizer interface {
	Authorize(context.Context, string, uint64) error
}

// Service only releases an entry after the current IAM User has an active
// ProfileLink to the target Testee. The task token is not an authorization.
type Service struct {
	reader Reader
	access AccessAuthorizer
}

func NewService(reader Reader, access AccessAuthorizer) *Service {
	return &Service{reader: reader, access: access}
}

func (s *Service) Resolve(ctx context.Context, userID, taskID, token string) (*Entry, error) {
	if s == nil || s.reader == nil || s.access == nil {
		return nil, testeeaccess.ErrAccessUnavailable
	}
	if userID == "" {
		return nil, testeeaccess.ErrAccessDenied
	}
	entry, err := s.reader.ResolveTaskEntry(ctx, taskID, token)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrInvalidEntry
	}
	testeeID, err := strconv.ParseUint(entry.TesteeID, 10, 64)
	if err != nil || testeeID == 0 {
		return nil, ErrInvalidEntry
	}
	if err := s.access.Authorize(ctx, userID, testeeID); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *Service) List(ctx context.Context, userID, testeeID string) ([]*Entry, error) {
	if s == nil || s.reader == nil || s.access == nil {
		return nil, testeeaccess.ErrAccessUnavailable
	}
	id, err := strconv.ParseUint(testeeID, 10, 64)
	if err != nil || id == 0 || userID == "" {
		return nil, testeeaccess.ErrAccessDenied
	}
	if err := s.access.Authorize(ctx, userID, id); err != nil {
		return nil, err
	}
	reader, ok := s.reader.(interface {
		ListParticipantTasks(context.Context, string) ([]*Entry, error)
	})
	if !ok {
		return nil, testeeaccess.ErrAccessUnavailable
	}
	entries, err := reader.ListParticipantTasks(ctx, testeeID)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry == nil || entry.TesteeID != testeeID {
			return nil, testeeaccess.ErrAccessUnavailable
		}
	}
	if err := s.access.Authorize(ctx, userID, id); err != nil {
		return nil, err
	}
	return entries, nil
}
