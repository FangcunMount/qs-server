//go:build integration

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestRetirementMongoNativeOwnedReplicaSet(t *testing.T) {
	in := ownedNativeInput(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	admin, err := connect(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if admin.Close(c) != nil {
			t.Error("owned admin release failed")
		}
	}()
	rawHead := nativeCommand(t, admin, "qs", bson.D{{Key: "find", Value: "schema_migrations"}, {Key: "filter", Value: bson.D{}}, {Key: "limit", Value: 2}, {Key: "singleBatch", Value: true}})
	reader := reader{c: admin, cursorReleased: true}
	heads, err := reader.batch(ctx, rawHead, "schema_migrations")
	if err != nil || len(heads) > 1 {
		t.Fatal("owned schema head invalid")
	}
	if len(heads) == 0 {
		nativeCommand(t, admin, "qs", bson.D{{Key: "insert", Value: "schema_migrations"}, {Key: "documents", Value: bson.A{bson.D{{Key: "_id", Value: "retirement-owned-head"}, {Key: "version", Value: int64(44)}, {Key: "dirty", Value: false}}}}})
		defer func() {
			nativeCommand(t, admin, "qs", bson.D{{Key: "delete", Value: "schema_migrations"}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "_id", Value: "retirement-owned-head"}}}, {Key: "limit", Value: 1}}}}})
		}()
	}
	nativeCommand(t, admin, "admin", bson.D{{Key: "createUser", Value: "retirement_readonly"}, {Key: "pwd", Value: in.Password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: "qs"}}}}})
	// This collection belongs exclusively to this already verified CI fixture.
	nativeCommand(t, admin, "qs", bson.D{{Key: "create", Value: "rm_outbox"}})
	defer func() { nativeCommand(t, admin, "qs", bson.D{{Key: "drop", Value: "rm_outbox"}}) }()
	nativeCommand(t, admin, "qs", bson.D{{Key: "insert", Value: "rm_outbox"}, {Key: "writeConcern", Value: bson.D{{Key: "w", Value: "majority"}, {Key: "wtimeout", Value: 5000}}}, {Key: "documents", Value: bson.A{
		bson.D{{Key: "state", Value: "published"}}, bson.D{{Key: "state", Value: "PUBLISHED"}}, bson.D{{Key: "state", Value: "published "}}, bson.D{{Key: "state", Value: nil}}, bson.D{{Key: "unrelated", Value: true}}, bson.D{{Key: "state", Value: "future"}}, bson.D{{Key: "state", Value: bson.A{"published"}}},
	}}})
	// Use an actual read-only account: the probe cannot insert/update/drop.
	limited := in
	limited.Account = "retirement_readonly"
	c, err := connect(ctx, limited)
	if err != nil {
		t.Fatal(err)
	}
	r := retirementProbe(ctx, c, strings.Repeat("a", 40))
	if !r.Complete || !r.PoolReleased || !r.CursorsReleased || *r.Rows != 7 || *r.Closed != 1 || *r.Other != 6 || *r.Null != 2 || r.DropAuthorized {
		t.Fatal(r)
	}
	c, err = connect(ctx, limited)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := c.Command(ctx, "qs", bson.D{{Key: "insert", Value: "rm_outbox"}, {Key: "documents", Value: bson.A{bson.D{{Key: "state", Value: "published"}}}}})
	if c.Close(ctx) != nil || writeErr == nil {
		t.Fatal("read-only fixture write guard failed")
	}
}
