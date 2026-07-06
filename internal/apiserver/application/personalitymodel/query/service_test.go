package query

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type fakeAlgorithmLister struct {
	algorithms []domain.Algorithm
}

func (f fakeAlgorithmLister) ListPublishedAlgorithms(context.Context) ([]domain.Algorithm, error) {
	return append([]domain.Algorithm(nil), f.algorithms...), nil
}

func TestGetCategoriesUsesPublishedAlgorithmLister(t *testing.T) {
	svc := NewQueryServiceWithAlgorithmLister(nil, fakeAlgorithmLister{
		algorithms: []domain.Algorithm{
			domain.AlgorithmMBTI,
			domain.AlgorithmSBTI,
			domain.AlgorithmBigFive,
		},
	})
	got, err := svc.GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(got.Categories) != 3 {
		t.Fatalf("categories = %#v", got.Categories)
	}
	if got.Categories[2].Label != "Big Five" {
		t.Fatalf("big five label = %s", got.Categories[2].Label)
	}
}

func TestGetCategoriesFallsBackToMBTIAndSBTI(t *testing.T) {
	svc := NewQueryService(nil)
	got, err := svc.GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(got.Categories) != 3 {
		t.Fatalf("categories = %#v", got.Categories)
	}
}
