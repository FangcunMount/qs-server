package migration

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Independent primary observations are not a distributed snapshot, Broker queue
// depth, consumer ACK, writer exclusion or permission to stop an owner.
type OfflineProtocolObservation struct {
	Phase                string                   `json:"phase"`
	ReadOnly             bool                     `json:"read_only"`
	Available            bool                     `json:"available"`
	Mutation             bool                     `json:"mutation"`
	RuntimeFrozen        bool                     `json:"runtime_frozen"`
	BrokerPendingKnown   bool                     `json:"broker_pending_known"`
	NoObservedUnresolved bool                     `json:"no_observed_unresolved"`
	Snapshot             OfflineSnapshot          `json:"snapshot"`
	MySQL                *OfflineOwnerObservation `json:"qs_mysql"`
	Mongo                *OfflineOwnerObservation `json:"qs_mongo"`
}

type OfflineOwnerObservation struct {
	Owner                string                          `json:"owner"`
	BindingSHA256        string                          `json:"binding_sha256"`
	Protocol             transaction.ProtocolObservation `json:"protocol"`
	NativeLimitActive    bool                            `json:"native_limit_active"`
	NoObservedUnresolved bool                            `json:"no_observed_unresolved"`
}

func unresolvedProtocol(p transaction.ProtocolObservation) (bool, error) {
	counts := []int64{p.ReservedExecutions, p.PreparingExecutions, p.ConfirmationGapBranches,
		p.UnagedConfirmationGapBranches, p.UntrackedAbortedBranches, p.UnobservedCommittedBranches, p.MetadataConflicts}
	if p.AsOf.IsZero() {
		return false, errors.New("protocol observation unavailable")
	}
	zero := true
	for _, n := range counts {
		if n < 0 {
			return false, errors.New("protocol observation invalid")
		}
		zero = zero && n == 0
	}
	for _, n := range []float64{p.PreparedExpiryAgeSeconds, p.OldestConfirmationObservationAgeSeconds} {
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return false, errors.New("protocol observation age invalid")
		}
		zero = zero && n == 0
	}
	return zero, nil
}

func observeOfflineOwner(ctx context.Context, b transaction.Binding, reader transaction.ObservationReader, native messagingruntime.NativeLimitReader) (*OfflineOwnerObservation, error) {
	if !b.Valid() || reader == nil || native == nil || native.Binding() != b {
		return nil, transaction.ErrBinding
	}
	// Borrow the SDK and host readers. No sender, Broker, lease or schema install.
	active, err := native.ReadNativeLimit(ctx)
	if err != nil {
		return nil, err
	}
	p, err := reader.ObserveProtocol(ctx)
	if err != nil {
		return nil, err
	}
	zero, err := unresolvedProtocol(p)
	if err != nil {
		return nil, err
	}
	hash, err := bindingHash(b)
	if err != nil {
		return nil, err
	}
	return &OfflineOwnerObservation{b.Owner, hash, p, active, zero && !active}, nil
}

// ObserveOfflineProtocol borrows the original pair and explicit owner bindings.
// Missing/invalid metadata is unavailable; the caller proves writer exclusion.
func ObserveOfflineProtocol(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig) (out OfflineProtocolObservation, resultErr error) {
	out.ReadOnly = true
	out.Phase = "input"
	if ctx == nil || db == nil || client == nil {
		return out, errors.New("bounded original pair required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out.Phase = "pair_inspection"
	out.Snapshot, resultErr = InspectOffline(ctx, db, client, cfg)
	if resultErr != nil {
		return out, resultErr
	}
	if (out.Snapshot.MySQLVersion != 110 && out.Snapshot.MySQLVersion != 111) || out.Snapshot.MongoVersion != 44 || out.Snapshot.TargetMySQLVersion != 111 || out.Snapshot.TargetMongoVersion != 44 {
		return out, errors.New("explicit M5 installed pair required")
	}
	out.Phase = "qs_mysql_binding"
	b := transaction.Binding{Owner: "qs.mysql"}
	if err := db.QueryRowContext(ctx, "SELECT store_id,writer_epoch FROM rm_tx_binding WHERE owner=?", []byte(b.Owner)).Scan(&b.StoreID, &b.WriterEpoch); err != nil {
		return out, err
	}
	s, err := txmysql.New(db, b)
	if err != nil {
		return out, err
	}
	nativeSQL, err := messagingruntime.NewSQLNativeLimits(db, b, "m3-1")
	if err != nil {
		return out, err
	}
	out.Phase = "qs_mysql_protocol"
	out.MySQL, err = observeOfflineOwner(ctx, b, s, nativeSQL)
	if err != nil {
		return out, err
	}
	out.Phase = "qs_mongo_binding"
	var m struct {
		StoreID     string `bson:"store_id"`
		WriterEpoch string `bson:"writer_epoch"`
	}
	documents := client.Database(cfg.MongoDatabase)
	if err = documents.Collection("rm_tx_binding").FindOne(ctx, bson.M{"_id": "qs.mongo"}).Decode(&m); err != nil {
		return out, err
	}
	b = transaction.Binding{Owner: "qs.mongo", StoreID: m.StoreID, WriterEpoch: m.WriterEpoch}
	ms, err := txmongo.New(documents, b)
	if err != nil {
		return out, err
	}
	nativeMongo, err := messagingruntime.NewMongoNativeLimits(documents, b, "m3-1")
	if err != nil {
		return out, err
	}
	out.Phase = "qs_mongo_protocol"
	out.Mongo, err = observeOfflineOwner(ctx, b, ms, nativeMongo)
	if err != nil {
		return out, err
	}
	out.Phase = "complete"
	out.Available = true
	out.NoObservedUnresolved = out.MySQL.NoObservedUnresolved && out.Mongo.NoObservedUnresolved
	return out, nil
}
