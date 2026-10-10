package main

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestComparisonMongoAbortRequiresUnambiguousActualAcknowledgement(t *testing.T) {
	for name, doc := range map[string]bson.D{
		"double": {{Key: "ok", Value: float64(1)}},
		"int32":  {{Key: "ok", Value: int32(1)}},
		"int64":  {{Key: "ok", Value: int64(1)}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, e := bson.Marshal(doc)
			if e != nil || !lifecycleMongoAbortAcknowledged(raw) {
				t.Fatal("native numeric success shape rejected")
			}
		})
	}
	for name, doc := range map[string]bson.D{
		"missing": {}, "string": {{Key: "ok", Value: "1"}}, "boolean": {{Key: "ok", Value: true}}, "failed": {{Key: "ok", Value: float64(0)}},
		"duplicate":        {{Key: "ok", Value: float64(1)}, {Key: "ok", Value: float64(0)}},
		"writeConcern":     {{Key: "ok", Value: float64(1)}, {Key: "writeConcernError", Value: bson.D{{Key: "code", Value: 64}}}},
		"nullWriteConcern": {{Key: "ok", Value: float64(1)}, {Key: "writeConcernError", Value: nil}},
		"writeErrors":      {{Key: "ok", Value: float64(1)}, {Key: "writeErrors", Value: bson.A{}}},
		"code":             {{Key: "ok", Value: float64(1)}, {Key: "code", Value: int32(13)}},
		"errmsg":           {{Key: "ok", Value: float64(1)}, {Key: "errmsg", Value: "unknown"}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, e := bson.Marshal(doc)
			if e != nil || lifecycleMongoAbortAcknowledged(raw) {
				t.Fatal("missing/ambiguous/error response became cleanup success")
			}
		})
	}
	if lifecycleMongoAbortAcknowledged(bson.Raw{0, 1}) {
		t.Fatal("malformed server response became cleanup success")
	}
}

func TestComparisonMongoAbortRejectsCanceledAndUnissuedNativeSession(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, ctx := range []context.Context{nil, canceled, t.Context()} {
		if lifecycleAbortComparisonMongo(ctx, new(mongo.Client), nil) == nil {
			t.Fatal("canceled/missing original session became cleanup evidence")
		}
	}
}
