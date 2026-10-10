package main

import (
	"bytes"
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// The pinned Session.AbortTransaction method intentionally ignores the actual
// abort command's Execute error. It can therefore update only local state while
// the server result remains unknown. A completed comparison requires this
// separate native response first, on its original real session/transaction.
// The host still performs ordinary Abort/EndSession afterwards for local state.
func lifecycleAbortComparisonMongo(ctx context.Context, client *mongo.Client, original mongo.Session) error {
	if ctx == nil || ctx.Err() != nil || client == nil || original == nil {
		return lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
	}
	deadline, bounded := ctx.Deadline()
	remaining := time.Until(deadline)
	if !bounded || remaining <= 0 || remaining > 10*time.Second {
		return lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
	}
	x, ok := original.(mongo.XSession) //nolint:staticcheck // Pinned native driver transaction identity; no session DTO.
	if !ok || x.ClientSession() == nil || !x.ClientSession().TransactionInProgress() || x.ClientSession().CurrentRc == nil || x.ClientSession().CurrentRc.Level != "snapshot" {
		return lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
	}
	id := append(bson.Raw(nil), original.ID()...)
	number := x.ClientSession().TxnNumber
	if len(id) == 0 || number <= 0 {
		return lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
	}
	// RunCommand binds its lsid/txnNumber/autocommit fields from this same real
	// SessionContext and verifies original client ownership. Do not manually
	// supply these fields, create a client/session, or hide the driver context.
	bound := mongo.NewSessionContext(ctx, original)
	var reply bson.Raw
	e := client.Database("admin").RunCommand(bound, bson.D{{Key: "abortTransaction", Value: 1}}, options.RunCmd().SetReadPreference(readpref.Primary())).Decode(&reply)
	if e != nil || ctx.Err() != nil || !lifecycleMongoAbortAcknowledged(reply) || !bytes.Equal(original.ID(), id) || x.ClientSession().TxnNumber != number || !x.ClientSession().TransactionInProgress() {
		return lifecycleError("lifecycle_acceptance_snapshot_cleanup_failed")
	}
	return nil
}

func lifecycleMongoAbortAcknowledged(reply bson.Raw) bool {
	if reply.Validate() != nil || reply.Lookup("writeConcernError").Type != 0 || reply.Lookup("writeErrors").Type != 0 || reply.Lookup("code").Type != 0 || reply.Lookup("errmsg").Type != 0 {
		return false
	}
	// A missing, wrong-typed, duplicate or ambiguous response is not success.
	elements, e := reply.Elements()
	if e != nil {
		return false
	}
	n := 0
	for _, element := range elements {
		if element.Key() == "ok" {
			n++
		}
	}
	if n != 1 {
		return false
	}
	v := reply.Lookup("ok")
	if n, ok := v.DoubleOK(); ok {
		return n == 1
	}
	if n, ok := v.Int32OK(); ok {
		return n == 1
	}
	if n, ok := v.Int64OK(); ok {
		return n == 1
	}
	return false
}
