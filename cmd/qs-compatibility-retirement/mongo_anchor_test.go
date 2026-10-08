package main

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func anchorRaw(t *testing.T, value any) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func anchorFixture(t *testing.T) (bson.Raw, bson.Raw) {
	t.Helper()
	hello := anchorRaw(t, bson.D{{Key: "setName", Value: "fixture-rs"}, {Key: "hosts", Value: bson.A{"node-b:27017", "node-a:27017"}}, {Key: "me", Value: "node-a:27017"}})
	response := anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "version", Value: 17}, {Key: "settings", Value: bson.D{{Key: "replicaSetId", Value: primitive.NewObjectID()}}}}}})
	return hello, response
}
func TestMongoAnchorExcludesMutablePrimaryAndMigrationGeneration(t *testing.T) {
	hello, response := anchorFixture(t)
	first, err := mongoAnchorFromMetadata(hello, response, "selected-business")
	if err != nil || !hashRE.MatchString(first) {
		t.Fatalf("anchor unavailable: %v", err)
	}
	elected := anchorRaw(t, bson.D{{Key: "setName", Value: "fixture-rs"}, {Key: "hosts", Value: bson.A{"node-a:27017", "node-b:27017"}}, {Key: "me", Value: "node-b:27017"}, {Key: "electionId", Value: primitive.NewObjectID()}})
	changed, err := mongoAnchorFromMetadata(elected, response, "selected-business")
	if err != nil || first != changed {
		t.Fatal("primary election changed stable anchor")
	}
	another, err := mongoAnchorFromMetadata(hello, response, "different-business")
	if err != nil || first == another {
		t.Fatal("selected namespace not bound")
	}
	otherResponse := anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "settings", Value: bson.D{{Key: "replicaSetId", Value: primitive.NewObjectID()}}}}}})
	another, err = mongoAnchorFromMetadata(hello, otherResponse, "selected-business")
	if err != nil || first == another {
		t.Fatal("replica-set identity not bound")
	}
	a := anchorRaw(t, bson.D{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte("1234567890abcdef")}}}}})
	b := anchorRaw(t, bson.D{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte("abcdef1234567890")}}}}})
	ga, ea := mongoMigrationGeneration(a)
	gb, eb := mongoMigrationGeneration(b)
	if ea != nil || eb != nil || ga == gb || !hashRE.MatchString(ga) {
		t.Fatal("generation UUID must be independently bound")
	}
}
func TestMongoAnchorRejectsUnsupportedOrAmbiguousMetadata(t *testing.T) {
	hello, response := anchorFixture(t)
	for _, tc := range []struct {
		name            string
		hello, response bson.Raw
		namespace       string
	}{
		{"standalone", anchorRaw(t, bson.D{{Key: "isWritablePrimary", Value: true}}), response, "selected"},
		{"router", anchorRaw(t, bson.D{{Key: "setName", Value: "fixture-rs"}, {Key: "msg", Value: "isdbgrid"}}), response, "selected"},
		{"different-set", anchorRaw(t, bson.D{{Key: "setName", Value: "other-rs"}}), response, "selected"},
		{"duplicate-set", anchorRaw(t, bson.D{{Key: "setName", Value: "fixture-rs"}, {Key: "setName", Value: "different"}}), response, "selected"},
		{"missing-config", hello, anchorRaw(t, bson.D{}), "selected"},
		{"missing-set-id", hello, anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "settings", Value: bson.D{}}}}}), "selected"},
		{"string-set-id", hello, anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "settings", Value: bson.D{{Key: "replicaSetId", Value: "not-an-object-id"}}}}}}), "selected"},
		{"zero-set-id", hello, anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "settings", Value: bson.D{{Key: "replicaSetId", Value: primitive.NilObjectID}}}}}}), "selected"},
		{"duplicate-id", hello, anchorRaw(t, bson.D{{Key: "config", Value: bson.D{{Key: "_id", Value: "fixture-rs"}, {Key: "settings", Value: bson.D{{Key: "replicaSetId", Value: primitive.NewObjectID()}, {Key: "replicaSetId", Value: primitive.NewObjectID()}}}}}}), "selected"},
		{"empty-namespace", hello, response, ""},
		{"over-budget-namespace", hello, response, strings.Repeat("x", 129)},
		{"malformed-bson", bson.Raw{1, 2, 3}, response, "selected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := mongoAnchorFromMetadata(tc.hello, tc.response, tc.namespace); err == nil || got != "" {
				t.Fatal("invalid metadata produced a trusted anchor")
			}
		})
	}
	for _, value := range []bson.D{
		{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 0, Data: make([]byte, 16)}}}}},
		{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: make([]byte, 15)}}}}},
		{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: make([]byte, 16)}}, {Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: make([]byte, 16)}}}}},
	} {
		if got, err := mongoMigrationGeneration(anchorRaw(t, value)); err == nil || got != "" {
			t.Fatal("invalid generation produced a trusted hash")
		}
	}
}
