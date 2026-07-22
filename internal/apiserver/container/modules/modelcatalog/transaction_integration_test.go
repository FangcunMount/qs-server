//go:build integration

package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	modelrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	questionnairerepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoReleasePairRollsBackEveryFailureBoundary(t *testing.T) {
	t.Parallel()
	_, db := mongodbtest.ReplicaSetDatabase(t)
	runner := modtx.NewMongoRunner(db)

	for _, tc := range []struct {
		name           string
		failAfterWrite int
		cancelCommit   bool
	}{
		{name: "questionnaire save failure", failAfterWrite: 1},
		{name: "model save failure", failAfterWrite: 2},
		{name: "commit failure", cancelCommit: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			code := fmt.Sprintf("PAIR-%d", time.Now().UnixNano())
			questionnaireCode := "Q-" + code
			questionnaire, err := domainquestionnaire.NewQuestionnaire(meta.NewCode(questionnaireCode), "Contract", domainquestionnaire.WithRevision(1))
			if err != nil {
				t.Fatal(err)
			}
			model, err := domainmodel.NewAssessmentModel(domainmodel.NewAssessmentModelInput{
				Code: code, Kind: domainmodel.KindScale, Algorithm: domainmodel.AlgorithmScaleDefault,
				ProductChannel: domainmodel.ProductChannelMedicalScale, Title: "Contract", Now: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			qRepo := questionnairerepo.NewRepository(db)
			mRepo := modelrepo.NewDraftRepository(db)

			ctx := t.Context()
			cancelCommit := func() {}
			if tc.cancelCommit {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancelCommit = cancel
				defer cancel()
			}
			writeCount := 0
			err = runner.WithinTransaction(ctx, func(txCtx context.Context) error {
				if err := qRepo.Create(txCtx, questionnaire); err != nil {
					return err
				}
				writeCount++
				if writeCount == tc.failAfterWrite {
					return errors.New("injected questionnaire repository failure")
				}
				if err := mRepo.Create(txCtx, model); err != nil {
					return err
				}
				writeCount++
				if writeCount == tc.failAfterWrite {
					return errors.New("injected model repository failure")
				}
				if tc.cancelCommit {
					cancelCommit()
				}
				return nil
			})
			if tc.cancelCommit && err == nil {
				t.Fatal("commit failure error = nil")
			}
			if !tc.cancelCommit && err == nil {
				t.Fatal("repository failure error = nil")
			}
			assertPairAbsent(t, db, code, questionnaireCode)
		})
	}
}

func assertPairAbsent(t *testing.T, db *mongo.Database, modelCode, questionnaireCode string) {
	t.Helper()
	modelCount, err := db.Collection("assessment_models").CountDocuments(t.Context(), bson.M{"code": modelCode})
	if err != nil {
		t.Fatal(err)
	}
	questionnaireCount, err := db.Collection("questionnaires").CountDocuments(t.Context(), bson.M{"code": questionnaireCode})
	if err != nil {
		t.Fatal(err)
	}
	if modelCount != 0 || questionnaireCount != 0 {
		t.Fatalf("half-published pair remains: models=%d questionnaires=%d", modelCount, questionnaireCount)
	}
}
