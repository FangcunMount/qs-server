//go:build integration

package modelcatalog

import (
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestDraftRepositoryConcurrentWritersUseRevisionCAS(t *testing.T) {
	t.Parallel()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repo := NewDraftRepository(db)
	now := time.Now().UTC()
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "MODEL-CAS", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault,
		ProductChannel: domain.ProductChannelMedicalScale, Title: "Initial", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(t.Context(), model); err != nil {
		t.Fatal(err)
	}
	left, err := repo.FindByCode(t.Context(), model.Code)
	if err != nil {
		t.Fatal(err)
	}
	right, err := repo.FindByCode(t.Context(), model.Code)
	if err != nil {
		t.Fatal(err)
	}
	if err := left.UpdateBasicInfo("Left", "", left.SubKind, left.Algorithm, left.ProductChannel, "", nil, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := right.UpdateBasicInfo("Right", "", right.SubKind, right.Algorithm, right.ProductChannel, "", nil, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, candidate := range []*domain.AssessmentModel{left, right} {
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- repo.Update(t.Context(), candidate)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for updateErr := range errs {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, domain.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("Update error = %v", updateErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}

	refreshed, err := repo.FindByCode(t.Context(), model.Code)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Revision() != 2 {
		t.Fatalf("revision = %d, want 2", refreshed.Revision())
	}
	if err := refreshed.UpdateBasicInfo("Retry", "", refreshed.SubKind, refreshed.Algorithm, refreshed.ProductChannel, "", nil, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(t.Context(), refreshed); err != nil {
		t.Fatalf("refreshed retry: %v", err)
	}
}
