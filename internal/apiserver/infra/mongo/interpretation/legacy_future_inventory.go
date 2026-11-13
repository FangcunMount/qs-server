package interpretation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	inventoryMaxRows  = 10000
	inventoryMaxBytes = 128 << 20
)

type LegacyFutureInventory struct {
	Executions      int `json:"owner_executions"`
	ReportTerminals int `json:"report_terminal_executions"`
	Prepared        int `json:"v1_future_prepared"`
	Committed       int `json:"v1_future_committed"`
	Aborted         int `json:"v1_future_aborted"`
	Inactive        int `json:"v1_future_inactive"`
}

type inventoryExecution struct {
	ID      []byte            `bson:"_id"`
	Request []byte            `bson:"request_key"`
	Plan    []byte            `bson:"plan"`
	Digest  []byte            `bson:"plan_digest"`
	Fence   uint64            `bson:"fence"`
	Epoch   string            `bson:"writer_epoch"`
	State   transaction.State `bson:"state"`
	Witness []struct {
		Reference []byte            `bson:"reference"`
		State     transaction.State `bson:"state"`
	} `bson:"witnesses"`
}

var errLegacyInventory = errors.New("report future inventory is incomplete")

// InventoryLegacyFutures reads all qs.mongo executions within fixed limits.
// It counts frozen plans and local witnesses independently of Broker outcomes.
func InventoryLegacyFutures(ctx context.Context, db *mongo.Database) (LegacyFutureInventory, error) {
	if db == nil || mongo.SessionFromContext(ctx) != nil {
		return LegacyFutureInventory{}, errLegacyInventory
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	db = db.Client().Database(db.Name(), options.Database().SetReadPreference(readpref.Primary()))
	before, err := legacyInventoryUUIDs(ctx, db)
	if err != nil {
		return LegacyFutureInventory{}, err
	}
	session, err := db.Client().StartSession()
	if err != nil {
		return LegacyFutureInventory{}, err
	}
	defer session.EndSession(context.WithoutCancel(ctx))
	if err := session.StartTransaction(options.Transaction().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Snapshot())); err != nil {
		return LegacyFutureInventory{}, err
	}
	sc := mongo.NewSessionContext(ctx, session)
	result, readErr := inventoryLegacyFutures(sc, db)
	cleanup, stop := context.WithTimeout(context.WithoutCancel(sc), 2*time.Second)
	defer stop()
	if err := errors.Join(readErr, session.AbortTransaction(cleanup)); err != nil {
		return LegacyFutureInventory{}, err
	}
	after, err := legacyInventoryUUIDs(ctx, db)
	if err != nil || before != after {
		return LegacyFutureInventory{}, errors.Join(errLegacyInventory, err)
	}
	return result, nil
}

func legacyInventoryUUIDs(ctx context.Context, db *mongo.Database) ([2][16]byte, error) {
	var ids [2][16]byte
	specs, err := db.ListCollectionSpecifications(ctx, bson.M{"name": bson.M{"$in": bson.A{"rm_tx_execution", "rm_tx_request"}}})
	if err != nil || len(specs) != len(ids) {
		return ids, errors.Join(errLegacyInventory, err)
	}
	for _, spec := range specs {
		i := 0
		switch spec.Name {
		case "rm_tx_execution":
		case "rm_tx_request":
			i = 1
		default:
			return ids, errLegacyInventory
		}
		if spec.Type != "collection" || spec.UUID == nil || spec.UUID.Subtype != 4 || len(spec.UUID.Data) != 16 || ids[i] != [16]byte{} {
			return ids, errLegacyInventory
		}
		copy(ids[i][:], spec.UUID.Data)
	}
	if ids[0] == [16]byte{} || ids[1] == [16]byte{} {
		return ids, errLegacyInventory
	}
	return ids, nil
}

func inventoryLegacyFutures(ctx context.Context, db *mongo.Database) (result LegacyFutureInventory, err error) {
	find := options.Find().SetHint("rm_tx_expiry").SetLimit(inventoryMaxRows + 1).SetBatchSize(8).
		SetProjection(bson.M{"request_key": 1, "plan": 1, "plan_digest": 1, "fence": 1, "writer_epoch": 1, "state": 1, "witnesses.reference": 1, "witnesses.state": 1})
	cursor, err := db.Collection("rm_tx_execution").Find(ctx, bson.M{"owner": "qs.mongo"}, find)
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		err = errors.Join(err, cursor.Close(cleanup))
	}()
	totalBytes := 0
	for cursor.Next(ctx) {
		totalBytes += len(cursor.Current)
		result.Executions++
		if result.Executions > inventoryMaxRows || totalBytes > inventoryMaxBytes {
			return LegacyFutureInventory{}, errLegacyInventory
		}
		var row inventoryExecution
		if err := cursor.Decode(&row); err != nil {
			return LegacyFutureInventory{}, err
		}
		if err := classifyInventoryExecution(ctx, db, row, &result); err != nil {
			return LegacyFutureInventory{}, err
		}
	}
	return result, cursor.Err()
}

func classifyInventoryExecution(ctx context.Context, db *mongo.Database, row inventoryExecution, result *LegacyFutureInventory) error {
	plan, err := transaction.DecodePlan(row.Plan)
	if err != nil {
		return err
	}
	input := plan.Input()
	digest := plan.Digest()
	key := transaction.ExecutionKey(input.Owner, input.OperationID, row.Epoch, row.Fence)
	requestKey := transaction.RequestFor(plan).KeyDigest()
	if input.Owner != "qs.mongo" || row.Fence == 0 || row.Fence > transaction.MaxFence || !bytes.Equal(digest[:], row.Digest) ||
		!bytes.Equal(key[:], row.ID) || !bytes.Equal(requestKey[:], row.Request) {
		return errLegacyInventory
	}
	if !strings.HasPrefix(input.IdempotencyKey, "report-terminal/") {
		return nil
	}
	raw, err := shared.Artifact(plan, "qs.mongo", "m3-1")
	if err != nil {
		return err
	}
	frozen, err := validateReportExecution(plan, raw)
	if err != nil {
		return err
	}
	result.ReportTerminals++
	if frozen.Version != 1 || frozen.Future == nil {
		return nil
	}
	if len(row.Witness) != 1 || row.Witness[0].State != row.State {
		return errLegacyInventory
	}
	ref, err := transaction.DecodeReference(row.Witness[0].Reference)
	if err != nil {
		return err
	}
	expected, err := plan.Reference(input.Branches[0].ID, row.Fence, row.Epoch)
	if err != nil || ref != expected {
		return errLegacyInventory
	}
	var request struct {
		Owner  string `bson:"owner"`
		Scope  string `bson:"scope"`
		Key    string `bson:"idempotency_key"`
		Hash   []byte `bson:"input_hash"`
		Active []byte `bson:"active_execution"`
	}
	if err := db.Collection("rm_tx_request").FindOne(ctx, bson.M{"_id": requestKey[:]}).Decode(&request); err != nil {
		return err
	}
	if request.Owner != input.Owner || request.Scope != input.Scope || request.Key != input.IdempotencyKey ||
		!bytes.Equal(request.Hash, input.InputHash[:]) || len(request.Active) != len(key) {
		return errLegacyInventory
	}
	if !bytes.Equal(request.Active, key[:]) {
		if row.State != transaction.Aborted {
			return errLegacyInventory
		}
		active, err := db.Collection("rm_tx_execution").CountDocuments(ctx, bson.M{"_id": request.Active, "owner": "qs.mongo", "request_key": requestKey[:], "fence": bson.M{"$gt": row.Fence}}, options.Count().SetLimit(1))
		if err != nil || active != 1 {
			return errors.Join(errLegacyInventory, err)
		}
		result.Inactive++
	}
	switch row.State {
	case transaction.Prepared:
		result.Prepared++
	case transaction.Committed:
		result.Committed++
	case transaction.Aborted:
		result.Aborted++
	default:
		return errLegacyInventory
	}
	return nil
}
