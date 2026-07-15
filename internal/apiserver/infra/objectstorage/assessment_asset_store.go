package objectstorage

import (
	"context"
	"errors"
	"fmt"

	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	assessmentasset "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentasset"
)

// AssessmentAssetStore adapts the shared OSS ObjectStore to the
// application-facing assessmentasset port.
type AssessmentAssetStore struct {
	store objectstorageport.ObjectStore
}

var _ assessmentasset.ObjectStore = (*AssessmentAssetStore)(nil)

func NewAssessmentAssetStore(store objectstorageport.ObjectStore) assessmentasset.ObjectStore {
	if store == nil {
		return nil
	}
	return &AssessmentAssetStore{store: store}
}

func (s *AssessmentAssetStore) Put(ctx context.Context, key, contentType string, body []byte) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("assessment asset store is not configured")
	}
	return s.store.Put(ctx, key, contentType, body)
}

func (s *AssessmentAssetStore) Get(ctx context.Context, key string) (*assessmentasset.ObjectReader, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("assessment asset store is not configured")
	}
	reader, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, objectstorageport.ErrObjectNotFound) {
			return nil, assessmentasset.ErrObjectNotFound
		}
		return nil, err
	}
	if reader == nil {
		return nil, assessmentasset.ErrObjectNotFound
	}
	return &assessmentasset.ObjectReader{
		Body:          reader.Body,
		ContentType:   reader.ContentType,
		ContentLength: reader.ContentLength,
		CacheControl:  reader.CacheControl,
	}, nil
}
