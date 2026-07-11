package main

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestChooseKeepPrefersGeneratedThenNewest(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	genAt := newer

	docs := []reportDoc{
		{
			ID:        mustOID("aaaaaaaaaaaaaaaaaaaaaaaa"),
			DomainID:  1,
			Status:    "failed",
			CreatedAt: newer,
			UpdatedAt: newer,
		},
		{
			ID:          mustOID("bbbbbbbbbbbbbbbbbbbbbbbb"),
			DomainID:    1,
			Status:      statusGenerated,
			CreatedAt:   older,
			UpdatedAt:   older,
			GeneratedAt: &genAt,
		},
		{
			ID:        mustOID("cccccccccccccccccccccccc"),
			DomainID:  1,
			Status:    "pending",
			CreatedAt: older,
			UpdatedAt: older,
		},
	}

	keep, extras := chooseKeep(docs)
	if keep.ID.Hex() != "bbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("keep=%s, want generated doc", keep.ID.Hex())
	}
	if len(extras) != 2 {
		t.Fatalf("extras=%d, want 2", len(extras))
	}
}

func TestPreferDocTiesBrokenByObjectID(t *testing.T) {
	ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	a := reportDoc{ID: mustOID("111111111111111111111111"), Status: "pending", CreatedAt: ts, UpdatedAt: ts}
	b := reportDoc{ID: mustOID("222222222222222222222222"), Status: "pending", CreatedAt: ts, UpdatedAt: ts}
	if !preferDoc(b, a) {
		t.Fatal("expected larger ObjectID hex to win when all else equal")
	}
}

func mustOID(hex string) primitive.ObjectID {
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		panic(err)
	}
	return id
}
