package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var retirementSourceSHA = "development"

type retirementReceipt struct {
	Source          string  `json:"source_sha"`
	Owner           string  `json:"owner"`
	ObservedAt      string  `json:"observed_at"`
	ReadOnly        bool    `json:"read_only"`
	Mutation        bool    `json:"mutation"`
	Complete        bool    `json:"inventory_complete"`
	DropAuthorized  bool    `json:"drop_authorized"`
	BusinessClosed  bool    `json:"business_responsibility_closed"`
	ExportVerified  bool    `json:"original_body_export_verified"`
	WritersExcluded bool    `json:"writers_excluded"`
	PoolReleased    bool    `json:"pool_released"`
	CursorsReleased bool    `json:"cursors_released"`
	Binding         string  `json:"database_binding_sha256"`
	Catalog         string  `json:"collection_catalog_sha256"`
	Head            *uint64 `json:"schema_head"`
	Rows            *uint64 `json:"rows"`
	Closed          *uint64 `json:"publication_closed_rows"`
	Other           *uint64 `json:"other_or_null_state_rows"`
	Null            *uint64 `json:"null_or_missing_state_rows"`
	ReadConcern     string  `json:"count_read_concern"`
	Error           string  `json:"error_phase"`
}

func retirementHash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Only the aggregation is a snapshot read. Catalog/head stability is checked
// separately; this does not attest a maintenance freeze or cross-read snapshot.
func retirementProbe(ctx context.Context, c commander, source string) (out retirementReceipt) {
	out = retirementReceipt{Source: source, Owner: "qs.mongo", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ReadOnly: true, ReadConcern: "snapshot", Error: "identity"}
	r := reader{c: c, cursorReleased: true}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out.PoolReleased = c.Close(closeCtx) == nil
		out.CursorsReleased = r.cursorReleased
		if !out.PoolReleased || !out.CursorsReleased {
			out.Complete = false
			out.Error = "release"
		}
	}()
	if !shaRE.MatchString(source) {
		return out
	}
	hello, err := r.command(ctx, "admin", bson.D{{Key: "hello", Value: 1}})
	if err != nil {
		return out
	}
	set, ok := hello.Lookup("setName").StringValueOK()
	if !ok || set == "" {
		return out
	}
	primary, ok := hello.Lookup("isWritablePrimary").BooleanOK()
	if !ok || !primary {
		return out
	}
	batch := func(command bson.D, collection string) ([]bson.Raw, error) {
		raw, err := r.command(ctx, "qs", command)
		if err != nil {
			return nil, err
		}
		return r.batch(ctx, raw, collection)
	}
	headCommand := bson.D{{Key: "find", Value: "schema_migrations"}, {Key: "filter", Value: bson.D{}}, {Key: "projection", Value: bson.D{{Key: "_id", Value: 0}, {Key: "version", Value: 1}, {Key: "dirty", Value: 1}}}, {Key: "limit", Value: 2}, {Key: "singleBatch", Value: true}, {Key: "batchSize", Value: 2}, {Key: "maxTimeMS", Value: 2000}}
	catalogCommand := bson.D{{Key: "listCollections", Value: 1}, {Key: "filter", Value: bson.D{{Key: "name", Value: "rm_outbox"}}}, {Key: "nameOnly", Value: false}, {Key: "cursor", Value: bson.D{{Key: "batchSize", Value: 2}}}, {Key: "maxTimeMS", Value: 2000}}
	out.Error = "schema_head"
	heads, err := batch(headCommand, "schema_migrations")
	if err != nil || len(heads) != 1 {
		return out
	}
	head, ok := integer(heads[0].Lookup("version"))
	dirty, valid := heads[0].Lookup("dirty").BooleanOK()
	if !ok || !valid || dirty {
		return out
	}
	out.Head = &head
	out.Error = "collection_identity"
	catalog, err := batch(catalogCommand, "$cmd.listCollections")
	if err != nil || len(catalog) != 1 {
		return out
	}
	typ, ok := catalog[0].Lookup("type").StringValueOK()
	subtype, uuid, valid := catalog[0].Lookup("info", "uuid").BinaryOK()
	if !ok || typ != "collection" || !valid || subtype != 4 || len(uuid) != 16 {
		return out
	}
	out.Catalog = retirementHash(catalog[0])
	out.Binding = retirementHash(append([]byte("qs\x00"+set+"\x00"), uuid...))
	out.Error = "state_count"
	stateType := bson.D{{Key: "$type", Value: "$state"}}
	closed := bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{stateType, "string"}}}, bson.D{{Key: "$eq", Value: bson.A{"$state", "published"}}}}}}
	null := bson.D{{Key: "$in", Value: bson.A{stateType, bson.A{"null", "missing"}}}}
	sumIf := func(condition bson.D) bson.D {
		return bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{condition, 1, 0}}}}}
	}
	pipeline := bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "rows", Value: bson.D{{Key: "$sum", Value: 1}}}, {Key: "closed", Value: sumIf(closed)}, {Key: "null", Value: sumIf(null)}}}}}
	command := bson.D{{Key: "aggregate", Value: "rm_outbox"}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{{Key: "batchSize", Value: 2}}}, {Key: "collation", Value: bson.D{{Key: "locale", Value: "simple"}}}, {Key: "readConcern", Value: bson.D{{Key: "level", Value: "snapshot"}}}, {Key: "allowDiskUse", Value: false}, {Key: "maxTimeMS", Value: 2000}}
	counts, err := batch(command, "rm_outbox")
	if err != nil || len(counts) > 1 {
		return out
	}
	var total, published, missing uint64
	if len(counts) == 1 {
		total, ok = integer(counts[0].Lookup("rows"))
		if !ok {
			return out
		}
		published, ok = integer(counts[0].Lookup("closed"))
		if !ok {
			return out
		}
		missing, ok = integer(counts[0].Lookup("null"))
		if !ok {
			return out
		}
	}
	if published > total || missing > total-published {
		return out
	}
	out.Error = "identity_recheck"
	afterHead, err := batch(headCommand, "schema_migrations")
	if err != nil || len(afterHead) != 1 || retirementHash(heads[0]) != retirementHash(afterHead[0]) {
		return out
	}
	afterCatalog, err := batch(catalogCommand, "$cmd.listCollections")
	if err != nil || len(afterCatalog) != 1 || out.Catalog != retirementHash(afterCatalog[0]) {
		return out
	}
	other := total - published
	out.Rows, out.Closed, out.Other, out.Null = &total, &published, &other, &missing
	out.Complete = true
	out.Error = "none"
	return out
}

func retirementConnection(ctx context.Context) (commander, error) {
	get := func(k string) string { return os.Getenv("QS_APISERVER_MONGODB_" + k) }
	// This fixed production namespace is independently reviewed; no arbitrary
	// database/collection/command or credentials enter command-line arguments.
	if get("DATABASE") != "qs" {
		return nil, errors.New("explicit reviewed namespace required")
	}
	cfg := database.MongoConfig{URL: get("URL"), Host: get("HOST"), Username: get("USERNAME"), Password: get("PASSWORD"), Database: "qs", ReplicaSet: get("REPLICA_SET")}
	if cfg.URL == "" && (cfg.Host == "" || cfg.Username == "") {
		return nil, errors.New("explicit application binding required")
	}
	// Preserve all explicitly configured transport flags; malformed values fail.
	for key, dst := range map[string]*bool{"DIRECT_CONNECTION": &cfg.DirectConnection, "USE_SSL": &cfg.UseSSL, "SSL_INSECURE_SKIP_VERIFY": &cfg.SSLInsecureSkipVerify, "SSL_ALLOW_INVALID_HOSTNAMES": &cfg.SSLAllowInvalidHostnames} {
		switch get(key) {
		case "":
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			return nil, errors.New("invalid application transport binding")
		}
	}
	cfg.SSLCAFile, cfg.SSLPEMKeyfile = get("SSL_CA_FILE"), get("SSL_PEM_KEYFILE")
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.BuildURL()).SetMinPoolSize(0).SetMaxPoolSize(1).SetRetryReads(false).SetConnectTimeout(2*time.Second).SetServerSelectionTimeout(2*time.Second).SetSocketTimeout(3*time.Second).SetAppName("qs-messaging-retirement-readonly"))
	if err != nil {
		return nil, errors.New("application connector unavailable")
	}
	return &mongoCommander{client: c}, nil
}

func runRetirement(w io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if !shaRE.MatchString(retirementSourceSHA) {
		return 1
	}
	c, err := retirementConnection(ctx)
	if err != nil {
		_ = json.NewEncoder(w).Encode(retirementReceipt{Source: retirementSourceSHA, Owner: "qs.mongo", ReadOnly: true, Error: "application_binding"})
		return 1
	}
	r := retirementProbe(ctx, c, retirementSourceSHA)
	if json.NewEncoder(w).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
