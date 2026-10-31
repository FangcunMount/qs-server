package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func retirementFixture() []bson.D {
	hello := fixtureSnapshot()[0]
	head := cursor(bson.A{bson.D{{Key: "version", Value: int64(44)}, {Key: "dirty", Value: false}}})
	catalog := cursor(bson.A{bson.D{{Key: "name", Value: "rm_outbox"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: testUUID}}}}})
	count := cursor(bson.A{bson.D{{Key: "rows", Value: int64(5)}, {Key: "closed", Value: int64(1)}, {Key: "null", Value: int64(2)}}})
	return []bson.D{hello, head, catalog, count, head, catalog}
}

func TestRetirementMongoCountsAreNotDeletionAuthority(t *testing.T) {
	f := &fakeCommander{responses: retirementFixture()}
	r := retirementProbe(context.Background(), f, strings.Repeat("a", 40))
	if !r.Complete || *r.Rows != 5 || *r.Closed != 1 || *r.Other != 4 || *r.Null != 2 || !r.PoolReleased || !r.CursorsReleased || r.DropAuthorized || r.BusinessClosed || r.ExportVerified || r.WritersExcluded || r.Mutation {
		t.Fatal(r)
	}
	if len(f.commands) != 6 || !f.closed {
		t.Fatal("unbounded or unreleased probe")
	}
	for _, c := range f.commands {
		switch c[0].Key {
		case "hello", "find", "listCollections", "aggregate":
		default:
			t.Fatal("unexpected command", c)
		}
	}
}

func TestRetirementMongoUnknownNeverBecomesEmpty(t *testing.T) {
	for failed := 0; failed < 6; failed++ {
		f := &fakeCommander{responses: retirementFixture(), errors: map[int]error{failed: errors.New("private connection information")}}
		r := retirementProbe(context.Background(), f, strings.Repeat("a", 40))
		if r.Complete || r.Rows != nil || r.DropAuthorized || !f.closed {
			t.Fatal(failed, r)
		}
	}
	f := &fakeCommander{responses: retirementFixture(), closeError: errors.New("release uncertain")}
	if r := retirementProbe(context.Background(), f, strings.Repeat("a", 40)); r.Complete || r.PoolReleased {
		t.Fatal(r)
	}
}

func TestRetirementMongoAbsenceAndCatalogHeadDriftFail(t *testing.T) {
	for _, index := range []int{1, 2, 4, 5} {
		f := &fakeCommander{responses: retirementFixture()}
		f.responses[index] = cursor(bson.A{})
		r := retirementProbe(context.Background(), f, strings.Repeat("a", 40))
		if r.Complete || r.Rows != nil {
			t.Fatal(index, r)
		}
	}
	f := &fakeCommander{responses: retirementFixture()}
	f.responses[3] = cursor(bson.A{})
	r := retirementProbe(context.Background(), f, strings.Repeat("a", 40))
	if !r.Complete || r.Rows == nil || *r.Rows != 0 {
		t.Fatal("present empty collection was unavailable", r)
	}
}
