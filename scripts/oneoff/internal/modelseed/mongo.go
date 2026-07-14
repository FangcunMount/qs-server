package modelseed

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

// MongoTransactionRunner keeps a one-off model seed atomic across norm, draft,
// and published snapshot collections. The target MongoDB must support
// multi-document transactions.
type MongoTransactionRunner struct {
	client *mongo.Client
}

type TransactionRunner interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

func NewMongoTransactionRunner(client *mongo.Client) MongoTransactionRunner {
	return MongoTransactionRunner{client: client}
}

func RunAtomically(ctx context.Context, runner TransactionRunner, fn func(context.Context) error) error {
	if runner == nil {
		return fmt.Errorf("transaction runner is nil")
	}
	return runner.WithinTransaction(ctx, fn)
}

func (r MongoTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if r.client == nil {
		return fmt.Errorf("mongo client is nil")
	}
	if fn == nil {
		return nil
	}
	session, err := r.client.StartSession()
	if err != nil {
		return fmt.Errorf("start mongo session: %w", err)
	}
	defer session.EndSession(ctx)

	txnOptions := options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority())
	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(txCtx)
	}, txnOptions)
	if err != nil {
		return fmt.Errorf("mongo seed transaction: %w", err)
	}
	return nil
}

type PublishedCollection interface {
	CountDocuments(context.Context, interface{}, ...*options.CountOptions) (int64, error)
	UpdateMany(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

// ActivePublishedState separates the intended legacy identity migration from
// unrelated questionnaire/model conflicts. Matching snapshots may have any
// historical kind or algorithm, but must keep the same model code and exact
// questionnaire binding.
type ActivePublishedState struct {
	MatchingCount                int64
	QuestionnaireOtherModelCount int64
	ModelOtherQuestionnaireCount int64
}

func InspectActivePublished(ctx context.Context, collection PublishedCollection, modelCode, questionnaireCode, questionnaireVersion string) (ActivePublishedState, error) {
	if collection == nil {
		return ActivePublishedState{}, fmt.Errorf("published collection is nil")
	}
	matching, err := collection.CountDocuments(ctx, matchingPublishedFilter(modelCode, questionnaireCode, questionnaireVersion))
	if err != nil {
		return ActivePublishedState{}, fmt.Errorf("count matching published snapshots: %w", err)
	}
	questionnaireOtherModel, err := collection.CountDocuments(ctx, bson.M{
		"status":                "published",
		"deleted_at":            nil,
		"questionnaire_code":    questionnaireCode,
		"questionnaire_version": questionnaireVersion,
		"model_code":            bson.M{"$ne": modelCode},
	})
	if err != nil {
		return ActivePublishedState{}, fmt.Errorf("count questionnaire conflicts: %w", err)
	}
	modelOtherQuestionnaire, err := collection.CountDocuments(ctx, bson.M{
		"status":     "published",
		"deleted_at": nil,
		"model_code": modelCode,
		"$nor": []bson.M{{
			"questionnaire_code":    questionnaireCode,
			"questionnaire_version": questionnaireVersion,
		}},
	})
	if err != nil {
		return ActivePublishedState{}, fmt.Errorf("count model conflicts: %w", err)
	}
	return ActivePublishedState{
		MatchingCount:                matching,
		QuestionnaireOtherModelCount: questionnaireOtherModel,
		ModelOtherQuestionnaireCount: modelOtherQuestionnaire,
	}, nil
}

func (s ActivePublishedState) ValidateReplacement(force, hasDraft bool, modelCode, questionnaireCode, questionnaireVersion string) error {
	if s.MatchingCount > 1 {
		return fmt.Errorf("found %d active snapshots for model %s and questionnaire %s@%s; expected at most one", s.MatchingCount, modelCode, questionnaireCode, questionnaireVersion)
	}
	if s.QuestionnaireOtherModelCount > 0 {
		return fmt.Errorf("questionnaire %s@%s is published by %d other model(s); refusing replacement", questionnaireCode, questionnaireVersion, s.QuestionnaireOtherModelCount)
	}
	if s.ModelOtherQuestionnaireCount > 0 {
		return fmt.Errorf("model %s has %d active snapshot(s) bound to another questionnaire version; refusing replacement", modelCode, s.ModelOtherQuestionnaireCount)
	}
	if force {
		return nil
	}
	if hasDraft {
		return fmt.Errorf("draft model %s already exists; pass --force only after backup and review", modelCode)
	}
	if s.MatchingCount > 0 {
		return fmt.Errorf("published model %s already exists for questionnaire %s@%s; pass --force only after backup and review", modelCode, questionnaireCode, questionnaireVersion)
	}
	return nil
}

func RetireMatchingPublished(ctx context.Context, collection PublishedCollection, modelCode, questionnaireCode, questionnaireVersion string, expected int64, now time.Time) error {
	if expected == 0 {
		return nil
	}
	if expected != 1 {
		return fmt.Errorf("refusing to retire %d published snapshots; expected exactly one", expected)
	}
	result, err := collection.UpdateMany(ctx, matchingPublishedFilter(modelCode, questionnaireCode, questionnaireVersion), bson.M{"$set": bson.M{
		"deleted_at": now,
		"updated_at": now,
		"status":     "unpublished",
	}})
	if err != nil {
		return fmt.Errorf("retire matching published snapshot: %w", err)
	}
	if result.MatchedCount != expected || result.ModifiedCount != expected {
		return fmt.Errorf("retire matching published snapshot: matched=%d modified=%d want=%d", result.MatchedCount, result.ModifiedCount, expected)
	}
	return nil
}

func matchingPublishedFilter(modelCode, questionnaireCode, questionnaireVersion string) bson.M {
	return bson.M{
		"status":                "published",
		"deleted_at":            nil,
		"model_code":            modelCode,
		"questionnaire_code":    questionnaireCode,
		"questionnaire_version": questionnaireVersion,
	}
}
