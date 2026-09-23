//go:build reliable_messaging_m4

// Package standardoutbox owns QS-specific governance for the SDK Mongo collection.
// The standard Outbox and replay ledger remain in the host business database.
package standardoutbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"go.mongodb.org/mongo-driver/bson"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

type replayItem struct {
	EventID              string `bson:"event_id"`
	ExpectedFailureCount uint64 `bson:"expected_failure_count"`
	Authorized           bool   `bson:"authorized"`
	Reason               string `bson:"reason"`
}

type replayHeader struct {
	InputHash []byte       `bson:"input_hash"`
	Items     []replayItem `bson:"items"`
}

// ReplayLedger uses an embedded ordered item ledger in one Mongo document.
// It starts its own session and transaction; SDK Appender/Store remain separate.
type ReplayLedger struct {
	db        *driver.Database
	outbox    *driver.Collection
	requests  *driver.Collection
	storeName string
}

func NewReplayLedger(db *driver.Database, storeName string) (*ReplayLedger, error) {
	if db == nil || storeName == "" {
		return nil, errors.New("database and store name required")
	}
	return &ReplayLedger{
		db: db, outbox: db.Collection("rm_outbox"), requests: db.Collection("qs_rm_replay_requests"),
		storeName: storeName,
	}, nil
}

func (l *ReplayLedger) Authorize(ctx context.Context, input request.ReplayRequest) ([]request.ReplayResult, error) {
	fingerprint, err := input.Fingerprint()
	if err != nil {
		return nil, err
	}
	if input.Store != l.storeName {
		return nil, errors.New("replay request targets another Outbox profile")
	}
	for _, target := range input.Targets {
		if target.ExpectedFailureCount > math.MaxInt64 {
			return nil, errors.New("failure count exceeds MongoDB signed integer range")
		}
	}
	session, err := l.db.Client().StartSession()
	if err != nil {
		return nil, err
	}
	defer session.EndSession(context.Background())
	key := replayKey(input)
	var results []request.ReplayResult
	txOptions := options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority())
	_, err = session.WithTransaction(ctx, func(sc driver.SessionContext) (interface{}, error) {
		var existing replayHeader
		readErr := l.requests.FindOne(sc, bson.M{"_id": key}).Decode(&existing)
		if readErr == nil {
			prior, err := replayResults(input, fingerprint[:], existing)
			if err != nil {
				return nil, err
			}
			results = prior
			return nil, nil
		}
		if !errors.Is(readErr, driver.ErrNoDocuments) {
			return nil, readErr
		}
		_, err := l.requests.InsertOne(sc, bson.M{
			"_id": key, "org_id": input.OrgID, "request_id": input.RequestID,
			"store_name": input.Store, "reason": input.Reason, "input_hash": fingerprint[:],
			"items": bson.A{}, "created_at": time.Now().UTC(),
		})
		if err != nil {
			return nil, err
		}
		items := make([]replayItem, 0, len(input.Targets))
		for _, target := range input.Targets {
			result, err := l.authorizeOne(sc, input.OrgID, input.RequestID, target)
			if err != nil {
				return nil, err
			}
			items = append(items, replayItem{
				EventID: target.EventID, ExpectedFailureCount: target.ExpectedFailureCount,
				Authorized: result.Authorized, Reason: result.Reason,
			})
		}
		if _, err := l.requests.UpdateOne(sc, bson.M{"_id": key}, bson.M{"$set": bson.M{"items": items}}); err != nil {
			return nil, err
		}
		results = make([]request.ReplayResult, len(items))
		for i, item := range items {
			results[i] = request.ReplayResult{EventID: item.EventID, Authorized: item.Authorized, Reason: item.Reason}
		}
		return nil, nil
	}, txOptions)
	if err == nil {
		return results, nil
	}
	// Commit may have succeeded even when the caller lost its acknowledgment.
	// Reconcile by stable request identity; never rerun an uncertain authorization.
	resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	prior, found, resolveErr := l.Resolve(resolveCtx, input)
	if resolveErr != nil {
		return nil, errors.Join(err, resolveErr)
	}
	if found {
		return prior, nil
	}
	return nil, err
}

// Resolve is a read-only recovery path for an unknown commit or lost response.
// Not found means the caller must keep the request pending, not blindly issue
// a fresh authorization under a different request ID.
func (l *ReplayLedger) Resolve(ctx context.Context, input request.ReplayRequest) ([]request.ReplayResult, bool, error) {
	fingerprint, err := input.Fingerprint()
	if err != nil {
		return nil, false, err
	}
	if input.Store != l.storeName {
		return nil, false, errors.New("replay request targets another Outbox profile")
	}
	var header replayHeader
	// Acknowledged transactions use majority writes; reconcile against a
	// majority view so a rolled-back or uncommitted request cannot look final.
	majorityRequests := l.db.Collection("qs_rm_replay_requests", options.Collection().SetReadConcern(readconcern.Majority()))
	err = majorityRequests.FindOne(ctx, bson.M{"_id": replayKey(input)}).Decode(&header)
	if errors.Is(err, driver.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	results, err := replayResults(input, fingerprint[:], header)
	return results, true, err
}

func replayKey(input request.ReplayRequest) string {
	return strconv.FormatInt(input.OrgID, 10) + ":" + input.RequestID
}

func replayResults(input request.ReplayRequest, fingerprint []byte, header replayHeader) ([]request.ReplayResult, error) {
	if !bytes.Equal(header.InputHash, fingerprint) {
		return nil, request.ErrReplayInputConflict
	}
	if len(header.Items) != len(input.Targets) {
		return nil, request.ErrReplayLedgerCorrupt
	}
	results := make([]request.ReplayResult, len(header.Items))
	for i, item := range header.Items {
		if item.EventID != input.Targets[i].EventID || item.ExpectedFailureCount != input.Targets[i].ExpectedFailureCount {
			return nil, request.ErrReplayLedgerCorrupt
		}
		results[i] = request.ReplayResult{EventID: item.EventID, Authorized: item.Authorized, Reason: item.Reason}
	}
	return results, nil
}

func (l *ReplayLedger) authorizeOne(sc driver.SessionContext, orgID int64, requestID string, target request.ReplayTarget) (request.ReplayResult, error) {
	result := request.ReplayResult{EventID: target.EventID}
	cursor, err := l.outbox.Find(sc, bson.M{"message_id": target.EventID}, options.Find().SetLimit(2))
	if err != nil {
		return result, err
	}
	var found []struct {
		ID           bson.D `bson:"_id"`
		Scope        string `bson:"scope"`
		State        string `bson:"state"`
		Code         string `bson:"last_error_code"`
		FailureCount uint64 `bson:"failure_count"`
		Version      uint64 `bson:"version"`
	}
	err = cursor.All(sc, &found)
	if err != nil {
		return result, err
	}
	if len(found) == 0 {
		result.Reason = "not_found"
		return result, nil
	}
	if len(found) > 1 {
		result.Reason = "ambiguous_identity"
		return result, nil
	}
	row := found[0]
	switch {
	case row.Scope != fmt.Sprintf("org:%d", orgID):
		result.Reason = "organization_mismatch"
	case row.State != "quarantined" || row.Code != "publish_unknown":
		result.Reason = "not_manual_required"
	case row.FailureCount != target.ExpectedFailureCount:
		result.Reason = "attempt_conflict"
	default:
		filter := bson.M{
			"_id": row.ID, "scope": row.Scope, "state": "quarantined",
			"last_error_code": "publish_unknown", "failure_count": int64(row.FailureCount),
			"version": int64(row.Version),
		}
		update := driver.Pipeline{bson.D{{Key: "$set", Value: bson.M{
			"state": bson.M{"$literal": "retry_wait"}, "next_attempt_at": "$$NOW",
			"claim_token": "$$REMOVE", "lease_until": "$$REMOVE",
			"manual_replay_request_id": bson.M{"$literal": requestID},
			"manual_replay_version":    bson.M{"$add": bson.A{"$version", 1}},
			"version":                  bson.M{"$add": bson.A{"$version", 1}},
			"updated_at":               "$$NOW",
		}}}}
		updated, err := l.outbox.UpdateOne(sc, filter, update)
		if err != nil {
			return result, err
		}
		if updated.MatchedCount != 1 {
			return result, errors.New("conditional replay update lost transaction snapshot")
		}
		result.Authorized = true
	}
	return result, nil
}
