package catalogreconcile

import (
	"context"
	"fmt"

	mongointerpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoStore adapts the Mongo catalog reconcile store to the application port.
type MongoStore struct {
	inner *mongointerpretation.CatalogReconcileStore
}

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{inner: mongointerpretation.NewCatalogReconcileStore(db)}
}

func (s *MongoStore) CountDrifts(ctx context.Context, filter Filter) (DriftCounts, error) {
	if s == nil || s.inner == nil {
		return DriftCounts{}, fmt.Errorf("catalog reconcile mongo store is not configured")
	}
	counts, err := s.inner.CountDrifts(ctx, mongointerpretation.CatalogReconcileFilter{
		OrgID:        filter.OrgID,
		SortAtAfter:  filter.SortAtAfter,
		SortAtBefore: filter.SortAtBefore,
	})
	if err != nil {
		return DriftCounts{}, err
	}
	return DriftCounts{
		Missing:             counts.Missing,
		Dangling:            counts.Dangling,
		AssociationMismatch: counts.AssociationMismatch,
		WrongWinner:         counts.WrongWinner,
	}, nil
}

var _ Store = (*MongoStore)(nil)
