//go:build integration

package aiexplanation_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	mongoai "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestAIExplanationGovernanceCatalogsPageStableScopedRecordsOnReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	retention := mongoai.RetentionPolicy{
		Version: "integration-v1", ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour,
	}
	evaluations, err := mongoai.NewPromptEvaluationRepository(db, retention)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := mongoai.NewProfileRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		runRecord, createErr := domainevaluation.NewRequested(
			meta.New(), integrationEvaluationRelease(fmt.Sprintf("catalog-%d", index)), 81,
			"integration-operator", "catalog paging", now.Add(time.Duration(index)*time.Minute),
		)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if createErr = runRecord.Cancel("integration-operator", "catalog terminal fixture", now.Add(time.Duration(index)*time.Minute+time.Second)); createErr != nil {
			t.Fatal(createErr)
		}
		if createErr = evaluations.Create(t.Context(), runRecord); createErr != nil {
			t.Fatal(createErr)
		}
	}
	other, err := domainevaluation.NewRequested(meta.New(), integrationEvaluationRelease("other-org"), 82, "other", "other organization", now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Cancel("other", "terminal", now.Add(4*time.Minute+time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = evaluations.Create(t.Context(), other); err != nil {
		t.Fatal(err)
	}

	evaluationStatus := domainevaluation.StatusCanceled
	first, next, err := evaluations.ListForReview(t.Context(), 81, &evaluationStatus, "", 2)
	if err != nil || len(first) != 2 || next == "" {
		t.Fatalf("first evaluation page = %d next=%q err=%v", len(first), next, err)
	}
	second, tail, err := evaluations.ListForReview(t.Context(), 81, &evaluationStatus, next, 2)
	if err != nil || len(second) != 1 || tail != "" || first[0].RunID == second[0].RunID {
		t.Fatalf("second evaluation page = %d tail=%q err=%v", len(second), tail, err)
	}
	for _, item := range append(first, second...) {
		if item.RequestedOrgID != 81 || item.Status != evaluationStatus {
			t.Fatalf("evaluation catalog crossed filter: org=%d status=%s", item.RequestedOrgID, item.Status)
		}
	}
	if _, _, err = evaluations.ListForReview(t.Context(), 82, &evaluationStatus, next, 2); !errors.Is(err, appevaluation.ErrReviewCatalogCursor) {
		t.Fatalf("cross-organization evaluation cursor error = %v", err)
	}

	suite, err := appevaluation.LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		definition := suite.ProfileFixture.Definition
		definition.ProfileID = fmt.Sprintf("integration-catalog-profile-%d", index)
		profileRecord, createErr := domainprofile.NewDraftForRelease(
			meta.New(), definition, "integration-operator", "catalog paging", now.Add(time.Duration(index)*time.Minute),
		)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if createErr = profiles.Save(t.Context(), profileRecord); createErr != nil {
			t.Fatal(createErr)
		}
	}
	profileStatus := domainprofile.StatusDraft
	profileFirst, profileNext, err := profiles.ListProfiles(t.Context(), &profileStatus, "", 2)
	if err != nil || len(profileFirst) != 2 || profileNext == "" {
		t.Fatalf("first Profile page = %d next=%q err=%v", len(profileFirst), profileNext, err)
	}
	profileSecond, profileTail, err := profiles.ListProfiles(t.Context(), &profileStatus, profileNext, 2)
	if err != nil || len(profileSecond) != 1 || profileTail != "" || profileFirst[0].ID() == profileSecond[0].ID() {
		t.Fatalf("second Profile page = %d tail=%q err=%v", len(profileSecond), profileTail, err)
	}
	published := domainprofile.StatusPublished
	if _, _, err = profiles.ListProfiles(t.Context(), &published, profileNext, 2); !errors.Is(err, appgovernance.ErrProfileCatalogCursor) {
		t.Fatalf("cross-status Profile cursor error = %v", err)
	}
}
