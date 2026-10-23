package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const InitialBindingSchema = "qs-initial-transaction-binding/v1"

// InitialBindingPlan authorizes one original owner only, never a distributed commit.
// The coordinator must keep every runtime stopped until both roots are verified.
type InitialBindingPlan struct {
	Schema            string              `json:"schema"`
	OperationID       string              `json:"operation_id"`
	Binding           transaction.Binding `json:"binding"`
	RouteRevision     string              `json:"route_revision"`
	ExpiresAt         time.Time           `json:"expires_at"`
	Original          OfflineSnapshot     `json:"original"`
	StoreSchemaSHA256 string              `json:"store_schema_sha256"`
}

type InitialBindingResult struct {
	Owner             string `json:"owner"`
	Outcome           string `json:"outcome"`
	Verified          bool   `json:"verified"`
	CutoverAuthorized bool   `json:"cutover_authorized"`
}

var initialSQLTables = []string{"rm_tx_binding", "rm_tx_request", "rm_tx_execution", "rm_tx_witness", "messaging_native_limit_notices"}
var initialMongoCollections = []string{"rm_tx_binding", "rm_tx_request", "rm_tx_execution", "messaging_native_limit_notices"}

func (p InitialBindingPlan) Validate(source string, now time.Time) error {
	if p.Schema != InitialBindingSchema || p.RouteRevision != "m3-1" || !validOfflineText(p.OperationID, 128) || !p.Binding.Valid() || len(p.Binding.Owner) > 128 || len(p.Binding.StoreID) > 128 || len(p.Binding.WriterEpoch) > 128 ||
		(p.Binding.Owner != "qs.mysql" && p.Binding.Owner != "qs.mongo") || !offlineSourceSHA.MatchString(source) || p.Original.SourceSHA != source || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(2*time.Hour)) || p.ExpiresAt.Format("-07:00") != "+00:00" {
		return errors.New("exact bounded initial owner plan required")
	}
	for _, v := range []string{p.Binding.StoreID, p.Binding.WriterEpoch} {
		if !validOfflineText(v, 128) {
			return errors.New("bounded original binding required")
		}
	}
	if !offlineDigest(p.Original.MigrationResourcesSHA256) || !offlineDigest(p.Original.MySQLIdentitySHA256) || !offlineDigest(p.Original.MongoClusterSHA256) || !validOfflineText(p.Original.MySQLDatabase, 64) || !validOfflineText(p.Original.MongoDatabase, 64) || p.Original.MySQLVersion == 0 || p.Original.MongoVersion == 0 || !offlineDigest(p.StoreSchemaSHA256) || p.Original.MySQLVersion != p.Original.TargetMySQLVersion || p.Original.MongoVersion != p.Original.TargetMongoVersion {
		return errors.New("complete original pair and schema digest required")
	}
	r, err := compiledMigrationResources()
	if err != nil || p.Original.TargetMySQLVersion != r.sql.head || p.Original.TargetMongoVersion != r.mongo.head || p.Original.MigrationResourcesSHA256 != r.sha256 {
		return errors.New("plan resources must match exact compiled schema pair")
	}
	return nil
}

func offlineDigest(v string) bool {
	raw, err := hex.DecodeString(v)
	return err == nil && len(raw) == 32 && v != fmt.Sprintf("%064d", 0) && hex.EncodeToString(raw) == v
}
func bindingHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func inspectBindingBase(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, b transaction.Binding, bound bool) (OfflineSnapshot, string, error) {
	if ctx == nil {
		return OfflineSnapshot{}, "", errors.New("bounded original context required")
	}
	if _, ok := ctx.Deadline(); !ok {
		return OfflineSnapshot{}, "", errors.New("bounded original context required")
	}
	if !b.Valid() || (b.Owner != "qs.mysql" && b.Owner != "qs.mongo") {
		return OfflineSnapshot{}, "", errors.New("explicit original owner required")
	}
	original, err := InspectOffline(ctx, db, client, cfg)
	if err != nil {
		return original, "", err
	}
	if original.MySQLVersion != original.TargetMySQLVersion || original.MongoVersion != original.TargetMongoVersion {
		return original, "", errors.New("install complete schema pair offline before binding")
	}
	var schema string
	if b.Owner == "qs.mysql" {
		schema, err = inspectInitialSQL(ctx, db, b, bound, false)
	} else {
		schema, err = inspectInitialMongo(ctx, client.Database(cfg.MongoDatabase, options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()).SetWriteConcern(writeconcern.Majority())), b, bound)
	}
	return original, schema, err
}

func PreviewInitialBinding(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, b transaction.Binding, operation string) (InitialBindingPlan, error) {
	original, schema, err := inspectBindingBase(ctx, db, client, cfg, b, false)
	if err != nil {
		return InitialBindingPlan{}, err
	}
	p := InitialBindingPlan{InitialBindingSchema, operation, b, "m3-1", time.Now().UTC().Add(time.Hour), original, schema}
	return p, p.Validate(cfg.ExpectedSourceSHA, time.Now().UTC())
}

func VerifyInitialBinding(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, p InitialBindingPlan) error {
	if err := p.Validate(cfg.ExpectedSourceSHA, time.Now().UTC()); err != nil {
		return err
	}
	original, schema, err := inspectBindingBase(ctx, db, client, cfg, p.Binding, true)
	if err != nil {
		return err
	}
	if original != p.Original || schema != p.StoreSchemaSHA256 {
		return errors.New("original installed binding identity/resources drifted")
	}
	return nil
}

// ApplyInitialBinding never retries a transaction or compensates a partial/unknown
// root. A separately approved invocation handles the other owner after readback.
func ApplyInitialBinding(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, p InitialBindingPlan, writersExcluded bool) (InitialBindingResult, error) {
	result := InitialBindingResult{Owner: p.Binding.Owner, Outcome: "not_attempted"}
	if !writersExcluded {
		return result, errors.New("explicit writer exclusion attestation required")
	}
	if err := p.Validate(cfg.ExpectedSourceSHA, time.Now().UTC()); err != nil {
		return result, err
	}
	original, schema, err := inspectBindingBase(ctx, db, client, cfg, p.Binding, false)
	if err != nil {
		return result, err
	}
	if original != p.Original || schema != p.StoreSchemaSHA256 {
		return result, errors.New("original pair/schema drifted after review")
	}
	if p.Binding.Owner == "qs.mysql" {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return result, err
		}
		defer func() { _ = tx.Rollback() }()
		current, err := inspectInitialSQL(ctx, tx, p.Binding, false, true)
		if err != nil {
			return result, err
		}
		if current != p.StoreSchemaSHA256 {
			return result, errors.New("original SQL schema changed")
		}
		if err = p.Validate(cfg.ExpectedSourceSHA, time.Now().UTC()); err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO rm_tx_binding(owner,store_id,writer_epoch,schema_version) VALUES(?,?,?,1)`, []byte(p.Binding.Owner), []byte(p.Binding.StoreID), []byte(p.Binding.WriterEpoch)); err != nil {
			return result, err
		}
		result.Outcome = "unknown"
		if err = tx.Commit(); err != nil {
			return result, err
		}
	} else {
		session, err := client.StartSession()
		if err != nil {
			return result, err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			session.EndSession(cleanup)
		}()
		err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
			if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()).SetReadPreference(readpref.Primary())); err != nil {
				return err
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = session.AbortTransaction(cleanup)
			}()
			target := client.Database(cfg.MongoDatabase, options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()).SetWriteConcern(writeconcern.Majority()))
			if err := initialMongoRows(sc, target, p.Binding, false); err != nil {
				return err
			}
			if err := p.Validate(cfg.ExpectedSourceSHA, time.Now().UTC()); err != nil {
				return err
			}
			if _, err := target.Collection("rm_tx_binding").InsertOne(sc, bson.M{"_id": p.Binding.Owner, "store_id": p.Binding.StoreID, "writer_epoch": p.Binding.WriterEpoch, "schema_version": 1}); err != nil {
				return err
			}
			result.Outcome = "unknown"
			return session.CommitTransaction(sc)
		})
		if err != nil {
			return result, err
		}
	}
	result.Outcome = "committed"
	if err = VerifyInitialBinding(ctx, db, client, cfg, p); err != nil {
		return result, err
	}
	result.Verified = true
	return result, nil
}

type initialSQLReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func inspectInitialSQL(ctx context.Context, db initialSQLReader, b transaction.Binding, bound, locked bool) (string, error) {
	ddl := map[string]string{}
	for _, table := range initialSQLTables {
		var name, body string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&name, &body); err != nil {
			return "", err
		}
		ddl[table] = body
		if table == "rm_tx_binding" && bound {
			rows, err := db.QueryContext(ctx, `SELECT owner,store_id,writer_epoch,schema_version FROM rm_tx_binding LIMIT 2`)
			if err != nil {
				return "", err
			}
			count := 0
			for rows.Next() {
				var owner, store, epoch string
				var version int
				if err = rows.Scan(&owner, &store, &epoch, &version); err != nil {
					_ = rows.Close()
					return "", err
				}
				count++
				if count != 1 || owner != b.Owner || store != b.StoreID || epoch != b.WriterEpoch || version != 1 {
					_ = rows.Close()
					return "", errors.New("original SQL binding mismatch")
				}
			}
			err = errors.Join(rows.Err(), rows.Close())
			if err != nil {
				return "", err
			}
			if count != 1 {
				return "", errors.New("original SQL binding missing")
			}
		} else {
			query := "SELECT 1 FROM `" + table + "` LIMIT 1"
			if locked {
				query += " FOR UPDATE"
			}
			var present int
			err := db.QueryRowContext(ctx, query).Scan(&present)
			if !errors.Is(err, sql.ErrNoRows) {
				if err != nil {
					return "", err
				}
				return "", errors.New("existing SQL protocol responsibility or binding")
			}
		}
	}
	return bindingHash(ddl)
}
func inspectInitialMongo(ctx context.Context, db *mongo.Database, b transaction.Binding, bound bool) (string, error) {
	store, err := txmongo.New(db, b)
	if err != nil {
		return "", err
	}
	if err = store.ValidateSchema(ctx); err != nil {
		return "", err
	}
	specs, err := db.ListCollectionSpecifications(ctx, bson.M{"name": bson.M{"$in": initialMongoCollections}})
	if err != nil {
		return "", err
	}
	if len(specs) != len(initialMongoCollections) {
		return "", errors.New("explicit Mongo protocol schemas required")
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	type collectionSchema struct {
		Spec    *mongo.CollectionSpecification
		Indexes []*mongo.IndexSpecification
	}
	all := []collectionSchema{}
	for _, spec := range specs {
		indexes, err := db.Collection(spec.Name).Indexes().ListSpecifications(ctx)
		if err != nil {
			return "", err
		}
		sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
		all = append(all, collectionSchema{spec, indexes})
	}
	if err = initialMongoRows(ctx, db, b, bound); err != nil {
		return "", err
	}
	return bindingHash(all)
}
func initialMongoRows(ctx context.Context, db *mongo.Database, b transaction.Binding, bound bool) error {
	for _, name := range initialMongoCollections {
		if name == "rm_tx_binding" && bound {
			cur, err := db.Collection(name).Find(ctx, bson.M{}, options.Find().SetLimit(2))
			if err != nil {
				return err
			}
			var rows []struct {
				Owner   string `bson:"_id"`
				Store   string `bson:"store_id"`
				Epoch   string `bson:"writer_epoch"`
				Version int    `bson:"schema_version"`
			}
			err = cur.All(ctx, &rows)
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].Owner != b.Owner || rows[0].Store != b.StoreID || rows[0].Epoch != b.WriterEpoch || rows[0].Version != 1 {
				return errors.New("original Mongo binding mismatch")
			}
		} else {
			err := db.Collection(name).FindOne(ctx, bson.M{}, options.FindOne().SetProjection(bson.M{"_id": 1})).Err()
			if !errors.Is(err, mongo.ErrNoDocuments) {
				if err != nil {
					return err
				}
				return errors.New("existing Mongo protocol responsibility or binding")
			}
		}
	}
	return nil
}
