package compatibilityretirementbackup

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoMetadataContextPreservesParentScope(t *testing.T) {
	type bindingKey struct{}
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), bindingKey{}, "original-operation"), deadline)
	defer cancel()
	// This test has no server/session proof. The genuine session-bearing
	// branch is exercised by TestMongoMetadataSnapshotNative.
	metadata := mongoMetadataContext(parent)
	if metadata != parent || metadata.Value(bindingKey{}) != "original-operation" || mongo.SessionFromContext(metadata) != nil {
		t.Fatal("ordinary metadata context changed its original scope")
	}
	actual, ok := metadata.Deadline()
	if !ok || !actual.Equal(deadline) || metadata.Done() != parent.Done() {
		t.Fatal("metadata context lost its original deadline/cancellation")
	}
	cancel()
	if !errors.Is(metadata.Err(), context.Canceled) {
		t.Fatal("metadata context survived cancellation of its original owner")
	}
}
