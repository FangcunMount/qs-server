//go:build integration

package questionnaire

import (
	"errors"
	"sync"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestRepositoryConcurrentWritersUseRevisionCAS(t *testing.T) {
	t.Parallel()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repo := NewRepository(db)
	questionnaire, err := domain.NewQuestionnaire(meta.NewCode("Q-CAS"), "Initial", domain.WithRevision(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(t.Context(), questionnaire); err != nil {
		t.Fatal(err)
	}
	left, err := repo.FindByCode(t.Context(), "Q-CAS")
	if err != nil {
		t.Fatal(err)
	}
	right, err := repo.FindByCode(t.Context(), "Q-CAS")
	if err != nil {
		t.Fatal(err)
	}
	if err := (domain.BaseInfo{}).UpdateTitle(left, "Left"); err != nil {
		t.Fatal(err)
	}
	if err := (domain.BaseInfo{}).UpdateTitle(right, "Right"); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, candidate := range []*domain.Questionnaire{left, right} {
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

	refreshed, err := repo.FindByCode(t.Context(), "Q-CAS")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.GetRevision() != 2 {
		t.Fatalf("revision = %d, want 2", refreshed.GetRevision())
	}
	if err := (domain.BaseInfo{}).UpdateTitle(refreshed, "Retry"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(t.Context(), refreshed); err != nil {
		t.Fatalf("refreshed retry: %v", err)
	}
}
