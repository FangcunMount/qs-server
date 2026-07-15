package reportstatus

import (
	"context"
	"errors"
	"fmt"
)

const (
	KindMedical     = "medical"
	KindPersonality = "personality"
	KindBehavior    = "behavior"
)

var (
	ErrInvalidKind      = errors.New("invalid assessment kind")
	ErrAssessmentAccess = errors.New("assessment access denied")
)

// KindReader 按 assessment kind 读取鉴权与当前状态。
type KindReader interface {
	Authorize(ctx context.Context, testeeID, assessmentID uint64) error
	CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*View, error)
}

// Resolver 统一 medical / personality / behavior 状态读取。
type Resolver struct {
	readers map[string]KindReader
}

func NewResolver(readers map[string]KindReader) *Resolver {
	if readers == nil {
		readers = map[string]KindReader{}
	}
	return &Resolver{readers: readers}
}

func (r *Resolver) Authorize(ctx context.Context, kind string, testeeID, assessmentID uint64) error {
	if r == nil {
		return fmt.Errorf("report status resolver is not configured")
	}
	reader, ok := r.readers[kind]
	if !ok || reader == nil {
		return ErrInvalidKind
	}
	return reader.Authorize(ctx, testeeID, assessmentID)
}

func (r *Resolver) CurrentStatus(ctx context.Context, kind string, testeeID, assessmentID uint64) (*View, error) {
	if err := r.Authorize(ctx, kind, testeeID, assessmentID); err != nil {
		return nil, err
	}
	reader := r.readers[kind]
	if reader == nil {
		return nil, ErrInvalidKind
	}
	return reader.CurrentStatus(ctx, testeeID, assessmentID)
}
