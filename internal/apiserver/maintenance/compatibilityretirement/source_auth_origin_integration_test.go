//go:build integration

package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"gorm.io/gorm"
)

func originNativeDBs(t *testing.T, empty bool) (*gorm.DB, *mongo.Client, *mongo.Database, MongoOwnerConfig) {
	t.Helper()
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	var name string
	if err := sqlDB.Raw("SELECT DATABASE()").Row().Scan(&name); err != nil {
		t.Fatal(err)
	}
	t.Log("owned_sql_database=" + name)
	t.Log("owned_mongo_database=" + db.Name())
	if err := db.CreateCollection(t.Context(), "domain_event_outbox"); err != nil {
		t.Fatal(err)
	}
	if !empty {
		for i, kind := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed"} {
			body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("origin-sql-%d", i+1)))
			row := fixtureSQLRow(t, body, fmt.Sprint(i+1))
			originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
		}
		for i := 1; i <= 8; i++ {
			row := aiFixtureRow(t, AIBridgeCommandSource, "start")
			id := fmt.Sprintf("10000000-0000-4000-8000-%012d", i)
			row[0], row[1] = []byte(id), []byte(id)
			row[3] = bytes.ReplaceAll(row[3], []byte(aiFixtureRequestID), []byte(id))
			row[4] = []byte(sourceSHA(row[3]))
			row[5] = []byte("1")
			if err := sqlDB.Exec("INSERT INTO ai_bridge_requests(request_id,request_hash,payload,organization_id,subject_id,testee_id) VALUES(?,?,?,?,?,?)", id, string(row[4]), string(row[3]), fixtureLargeID, "fixture-subject", "18446744073709551615").Error; err != nil {
				t.Fatal("owned request fixture failed")
			}
			originNativeInsertSQL(t, sqlDB, AIBridgeCommandSource, row)
		}
		for i, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
			body := bytes.ReplaceAll(wireFixture(t, kind), []byte("original-event-1"), []byte(fmt.Sprintf("origin-mongo-%d", i+1)))
			if _, err := db.Collection("domain_event_outbox").InsertOne(t.Context(), fixtureMongoRow(t, body, int64(i+1))); err != nil {
				t.Fatal(err)
			}
		}
	}
	return sqlDB, client, db, config
}
func originNativeInsertSQL(t *testing.T, db *gorm.DB, name string, row [][]byte) {
	t.Helper()
	var columns []string
	if name == "domain_event_outbox" {
		for _, c := range sqlSourceColumns {
			columns = append(columns, sourceOriginQuote(c.name))
		}
	} else {
		for _, c := range aiFixtureColumns(name) {
			columns = append(columns, sourceOriginQuote(*c[0]))
		}
	}
	args := make([]any, len(row))
	for i, cell := range row {
		if cell != nil {
			args[i] = string(cell)
		}
	}
	if err := db.Exec("INSERT INTO "+sourceOriginQuote(name)+"("+strings.Join(columns, ",")+") VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(row)), ",")+")", args...).Error; err != nil {
		t.Fatal("fixture row insert failed")
	}
}
func originNativeCopies(t *testing.T, ctx context.Context, tx *gorm.DB, global *MongoResponsibilitySnapshot) authFixture {
	t.Helper()
	var result authFixture
	limits := DefaultSourceOriginLimits()
	for i, name := range []string{"domain_event_outbox", AIBridgeCommandSource, AILegacyCommandSource} {
		boundary, columns, key, err := sourceOriginSQLMetadata(ctx, tx, SourceCopyExpectation{Boundary: SourceBoundary{Name: name}}, limits)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, len(columns))
		for j, c := range columns {
			names[j] = "CAST(" + sourceOriginQuote(*c[0]) + " AS BINARY)"
		}
		rows, err := sourceOriginSQLQuery(ctx, tx, limits, "SELECT "+strings.Join(names, ",")+" FROM "+sourceOriginQuote(name)+" FORCE INDEX (PRIMARY) ORDER BY "+sourceOriginQuote(key))
		if err != nil {
			t.Fatal(err)
		}
		var file bytes.Buffer
		if err = json.NewEncoder(&file).Encode(sqlSourceHeader{Protocol: SQLSourceProtocol, Boundary: boundary, Columns: columns}); err != nil {
			t.Fatal(err)
		}
		columnJSON, err := json.Marshal(columns)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(columnJSON)
		input := independentFrame([]byte(hex.EncodeToString(sum[:])), false)
		size := uint64(0)
		for _, row := range rows {
			encoded := make([]*string, len(row))
			for j, cell := range row {
				var raw []byte
				if cell != nil {
					raw = []byte(*cell)
					encoded[j] = strptr(base64.StdEncoding.EncodeToString(raw))
				}
				input = append(input, independentFrame(raw, cell == nil)...)
				size += uint64(len(raw))
			}
			if err = json.NewEncoder(&file).Encode(encoded); err != nil {
				t.Fatal(err)
			}
		}
		hash := sha256.Sum256(input)
		result.raw[i] = file.Bytes()
		result.expected[i] = SourceCopyExpectation{Boundary: boundary, DataHash: hex.EncodeToString(hash[:]), Records: uint64(len(rows)), Bytes: size}
	}
	boundary, _, err := sourceOriginMongoMetadata(ctx, global, limits)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := global.db.Collection("domain_event_outbox").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}).SetHint("_id_"))
	if err != nil {
		t.Fatal(err)
	}
	var file bytes.Buffer
	input := []byte{}
	count, size := uint64(0), uint64(0)
	for cursor.Next(ctx) {
		raw := cursor.Current
		var prefix [8]byte
		binary.BigEndian.PutUint64(prefix[:], uint64(len(raw)))
		file.Write(prefix[:])
		file.Write(raw)
		input = append(input, independentFrame(raw, false)...)
		count++
		size += uint64(len(raw))
	}
	readErr, closeErr := cursor.Err(), cursor.Close(ctx)
	if readErr != nil || closeErr != nil {
		t.Fatal("source BSON read failed")
	}
	hash := sha256.Sum256(input)
	result.raw[3] = file.Bytes()
	result.expected[3] = SourceCopyExpectation{Boundary: boundary, DataHash: hex.EncodeToString(hash[:]), Records: count, Bytes: size}
	return result
}

// Transactions, session and all lifecycle decisions remain owned by this test
// host. Capture first then end it normally; a later fresh scope does not need
// the first long snapshot to stay active.
func originNativeEpoch(t *testing.T, sqlDB *gorm.DB, db *mongo.Database, config MongoOwnerConfig, session mongo.Session, fn func(context.Context, *sqlevaluation.SQLHistoricalResponsibilityCycle, *MongoResponsibilitySnapshot, *gorm.DB) error) error {
	t.Helper()
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot())); err != nil {
		t.Fatal(err)
	}
	mongoCtx := mongo.NewSessionContext(t.Context(), session)
	err := sqlDB.Transaction(func(tx *gorm.DB) error {
		ctx := mongo.NewSessionContext(hostmysql.WithTx(mongoCtx, tx), session)
		var uuid, name string
		if e := tx.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &name); e != nil {
			return e
		}
		cycle, e := sqlevaluation.PrepareSQLHistoricalResponsibilityCycle(ctx, mongoOwnerHashParts("mysql_database_identity_v1", uuid, name), sqlevaluation.DefaultSQLResponsibilityLimits())
		if e != nil {
			return e
		}
		global, e := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
		if e != nil {
			return e
		}
		return fn(ctx, cycle, global, tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e := session.AbortTransaction(t.Context()); e != nil {
		t.Fatal(e)
	}
	return err
}
func TestSourceOriginNativeCompleteFourSourcesAndActualSameSessionNewTransaction(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			sqlDB, client, db, config := originNativeDBs(t, empty)
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			var fixture authFixture
			var binding *OriginCopyBinding
			var first *SourceOriginEpoch
			err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
				fixture = originNativeCopies(t, ctx, tx, global)
				copies, e := VerifySourceCopies(ctx, fixture.inputs())
				if e != nil {
					return e
				}
				binding, e = BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
				if e != nil {
					return e
				}
				binding.limits.PageRows = 2
				first, e = PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
				if e != nil {
					return e
				}
				if _, e = first.Recheck(ctx, cycle, global, originTestReaders(fixture)); e != ErrSourceOriginFresh {
					t.Fatal("same actual transaction accepted", e)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			// Prove host transaction ended without the primitive stealing pool/session.
			var n int
			if err = sqlDB.Raw("SELECT 1").Row().Scan(&n); err != nil || n != 1 {
				t.Fatal("host pool closed")
			}
			err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
				proof, e := first.Recheck(ctx, cycle, global, originTestReaders(fixture))
				if e != nil {
					return e
				}
				r := proof.Report()
				if !r.ActualOriginMatched || !r.IndependentEpochRechecked || r.DropReady || r.CASAuthorized || r.BusinessClosureVerified || !r.FirstAuthMetadataContinuityUnproven || !r.IndependentApprovalRequired {
					t.Fatal("incorrect origin authority")
				}
				if empty {
					for _, receipt := range r.Sources {
						if receipt.Records != 0 || !receipt.Complete {
							t.Fatal("empty source not fully covered")
						}
					}
				} else if r.Sources[0].Records != 4 || r.Sources[1].Records != 8 || r.Sources[2].Records != 0 || r.Sources[3].Records != 2 {
					t.Fatal("four-source coverage mismatch")
				}
				if first.transaction.number == global.txn.number || string(first.transaction.session) != string(global.txn.session) {
					t.Fatal("test did not exercise same Lsid new actual TxnNumber")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestSourceOriginNativeFreshRejectsActualSourceMetadataAndIdentityDrift(t *testing.T) {
	for _, kind := range []string{"sql_null", "sql_payload", "sql_upper", "sql_schema", "sql_head", "mongo_bson_order", "mongo_upper", "mongo_mixed_pk", "mongo_schema", "mongo_uuid", "sql_database_identity", "file_tamper"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB, client, db, config := originNativeDBs(t, false)
			session, err := client.StartSession()
			if err != nil {
				t.Fatal(err)
			}
			defer session.EndSession(t.Context())
			var fixture authFixture
			var first *SourceOriginEpoch
			err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
				fixture = originNativeCopies(t, ctx, tx, global)
				copies, e := VerifySourceCopies(ctx, fixture.inputs())
				if e != nil {
					return e
				}
				binding, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
				if e != nil {
					return e
				}
				first, e = PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
				return e
			})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "sql_null":
				err = sqlDB.Exec("UPDATE domain_event_outbox SET last_error='' WHERE id=1").Error
			case "sql_payload":
				err = sqlDB.Exec("UPDATE domain_event_outbox SET payload_json=REPLACE(payload_json,'q-code','q-other') WHERE id=1").Error
			case "sql_upper":
				row := fixtureSQLRow(t, bytes.ReplaceAll(wireFixture(t, "evaluation.failed"), []byte("original-event-1"), []byte("origin-extra")), "5")
				originNativeInsertSQL(t, sqlDB, "domain_event_outbox", row)
			case "sql_schema":
				err = sqlDB.Exec("ALTER TABLE domain_event_outbox ADD COLUMN source_extra VARCHAR(1) NULL").Error
			case "sql_head":
				err = sqlDB.Exec("UPDATE schema_migrations SET dirty=1").Error
			case "mongo_bson_order":
				var old bson.D
				if err = db.Collection("domain_event_outbox").FindOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}}).Decode(&old); err == nil {
					for i := range old {
						if old[i].Key == "event_id" {
							old[i], old[len(old)-1] = old[len(old)-1], old[i]
							break
						}
					}
					_, err = db.Collection("domain_event_outbox").ReplaceOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}}, old)
				}
			case "mongo_upper":
				_, err = db.Collection("domain_event_outbox").InsertOne(t.Context(), fixtureMongoRow(t, bytes.ReplaceAll(wireFixture(t, "answersheet.submitted"), []byte("original-event-1"), []byte("origin-extra")), int64(3)))
			case "mongo_mixed_pk":
				_, err = db.Collection("domain_event_outbox").InsertOne(t.Context(), fixtureMongoRow(t, bytes.ReplaceAll(wireFixture(t, "answersheet.submitted"), []byte("original-event-1"), []byte("origin-extra")), "other-pk"))
			case "mongo_schema":
				_, err = db.Collection("domain_event_outbox").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "event_id", Value: 1}}})
			case "mongo_uuid":
				err = db.Collection("domain_event_outbox").Drop(t.Context())
				if err == nil {
					err = db.CreateCollection(t.Context(), "domain_event_outbox")
				}
			case "sql_database_identity":
				sqlDB = mongoLocalSQLFixture(t)
				var changedName string
				if e := sqlDB.Raw("SELECT DATABASE()").Row().Scan(&changedName); e != nil {
					t.Fatal(e)
				}
				t.Log("owned_sql_database=" + changedName)
			case "file_tamper":
				fixture.raw[0] = append(append([]byte(nil), fixture.raw[0]...), []byte("{}\n")...)
			}
			if err != nil {
				t.Fatal("owned mutation failed", err)
			}
			err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, _ *gorm.DB) error {
				_, e := first.Recheck(ctx, cycle, global, originTestReaders(fixture))
				return e
			})
			if err == nil {
				t.Fatal("changed origin accepted")
			}
		})
	}
}
func TestSourceOriginNativeBorrowedReadOnlyScopeAndRollbackOwnership(t *testing.T) {
	sqlDB, client, db, config := originNativeDBs(t, false)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		fixture := originNativeCopies(t, ctx, tx, global)
		copies, e := VerifySourceCopies(ctx, fixture.inputs())
		if e != nil {
			return e
		}
		binding, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		if _, e = PrepareSourceOriginEpoch(context.Background(), binding, cycle, global, originTestReaders(fixture)); e == nil {
			t.Fatal("hostless scope accepted")
		}
		epoch, e := PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
		if e != nil {
			return e
		}
		if e = epoch.validate(ctx); e != nil {
			return e
		}
		return io.EOF
	})
	if err != io.EOF {
		t.Fatal("host callback error not preserved", err)
	}
	var count int64
	if err = sqlDB.Table("ai_bridge_commands").Count(&count).Error; err != nil || count != 8 {
		t.Fatal("borrowed origin scan changed source")
	}
	if err = client.Ping(t.Context(), nil); err != nil {
		t.Fatal("borrowed client closed")
	}
	// Avoid time-based authority: the immutable captured transaction identity is
	// what distinguishes epochs, while TTL only caps the proof's use lifetime.
}

func TestSourceOriginNativeOriginalMongoTransactionRejectedWithActualNewSQL(t *testing.T) {
	sqlDB, client, db, config := originNativeDBs(t, false)
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	err = originNativeEpoch(t, sqlDB, db, config, session, func(ctx context.Context, cycle *sqlevaluation.SQLHistoricalResponsibilityCycle, global *MongoResponsibilitySnapshot, tx *gorm.DB) error {
		fixture := originNativeCopies(t, ctx, tx, global)
		copies, e := VerifySourceCopies(ctx, fixture.inputs())
		if e != nil {
			return e
		}
		binding, e := BindOriginCopies(ctx, copies, fixture.inputs(), DefaultSourceOriginLimits())
		if e != nil {
			return e
		}
		first, e := PrepareSourceOriginEpoch(ctx, binding, cycle, global, originTestReaders(fixture))
		if e != nil {
			return e
		}
		return sqlDB.Transaction(func(newTx *gorm.DB) error {
			nextCtx := mongo.NewSessionContext(hostmysql.WithTx(ctx, newTx), session)
			nextCycle, e := sqlevaluation.PrepareSQLHistoricalResponsibilityCycle(nextCtx, cycle.Report().DatabaseIdentitySHA256, sqlevaluation.DefaultSQLResponsibilityLimits())
			if e != nil {
				return e
			}
			if nextCycle.Report().CycleID == first.sqlCycleID || newTx.Statement.ConnPool == first.sqlConnection {
				t.Fatal("test did not borrow an actual different SQL transaction")
			}
			if _, e = first.Recheck(nextCtx, nextCycle, global, originTestReaders(fixture)); e != ErrSourceOriginFresh {
				t.Fatal("original Mongo actual snapshot accepted despite a fresh SQL transaction", e)
			}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	})
	if err != nil {
		t.Fatal(err)
	}
}
