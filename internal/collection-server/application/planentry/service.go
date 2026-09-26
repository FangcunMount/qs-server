package planentry

import (
	"context"
	"errors"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/testeeaccess"
)

var ErrInvalidEntry = errors.New("plan task entry not found")

type Entry struct {
	TaskID    string
	TesteeID  string
	ScaleCode string
	ExpiresAt string
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
