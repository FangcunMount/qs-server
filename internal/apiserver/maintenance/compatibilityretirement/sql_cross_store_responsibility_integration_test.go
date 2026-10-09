//go:build integration

package retirement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func crossMongoNativePair(t *testing.T, sqlDB *gorm.DB, client *mongo.Client, db *mongo.Database, config MongoOwnerConfig, sources []*VerifiedSourceEvent, fn func(context.Context, *SQLCrossStoreResponsibilityCatalog, *SQLBusinessOwnerBatch, *MongoHistoricalOwnerBatch) error) error {
	t.Helper()
	var uuid, name string
	if err := sqlDB.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&uuid, &name); err != nil {
		t.Fatal(err)
	}
	expected := mongoOwnerHashParts("mysql_database_identity_v1", uuid, name)
	return mongoCycleNativeTx(t, client, func(mc mongo.SessionContext) error {
		return sqlDB.Transaction(func(tx *gorm.DB) error {
			ctx := mongo.NewSessionContext(hostmysql.WithTx(mc, tx), mongo.SessionFromContext(mc))
			current, err := PrepareSQLResponsibilitySnapshot(ctx, expected, sqlevaluation.DefaultSQLResponsibilityLimits())
			if err != nil {
				return err
			}
			catalog, err := PrepareSQLCrossStoreResponsibilityCatalog(ctx, current, sqlevaluation.DefaultSQLCrossStoreLimits())
			if err != nil {
				return err
			}
			selectors, err := MongoHistoricalSQLBatchSelectors(sources)
			if err != nil {
				return err
			}
			owners, err := PrepareSQLBusinessOwnerBatch(ctx, current, selectors, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			global, err := PrepareMongoResponsibilitySnapshot(ctx, db, config, mongoCycleTestLimits())
			if err != nil {
				return err
			}
			batch, err := PrepareMongoHistoricalOwnerBatch(ctx, global, owners.facts, sources, DefaultMongoHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			return fn(ctx, catalog, owners, batch)
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	})
}

func crossMongoNativeWire(t *testing.T, db *gorm.DB, evt event.DomainEvent, kind, state, suffix string, change func([]byte) []byte) {
	t.Helper()
	raw, err := domainwire.EncodeEvent(evt)
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		raw = change(raw)
	}
	if kind == "held" {
		var replayed any
		if state == "replayed" {
			replayed = time.Now().UTC().Truncate(time.Millisecond)
		}
		err = db.Exec("INSERT INTO retry_event_hold(event_id,message_id,org_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at,status,retry_disposition,replayed_at) VALUES(?,?,7,'nsq','native-cross-topic','native-cross-channel',?,1,'native fixture',UTC_TIMESTAMP(3),?,NULL,?)", evt.EventID(), "delivery-"+suffix, string(raw), state, replayed).Error
	} else {
		err = db.Exec("INSERT INTO event_delivery_dead_letter(message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,failed_at) VALUES(?,?,7,'nsq','native-cross-topic','native-cross-channel',1,?,?,UTC_TIMESTAMP(3))", "dead-"+suffix, evt.EventID(), string(raw), state).Error
	}
	if err != nil {
		t.Fatal(err)
	}
}

func crossMongoNativeReplay(t *testing.T, db *gorm.DB, eventID, store string, authorized bool, badHash bool) {
	t.Helper()
	r := standard.ReplayRequest{OrgID: 7, RequestID: "native-mongo-cross-replay", Store: store, Reason: "native real full replay input", Targets: []standard.ReplayTarget{{EventID: eventID, ExpectedFailureCount: 3}}}
	hash, err := r.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if badHash {
		hash[0] ^= 255
	}
	if err = db.Exec("INSERT INTO qs_rm_replay_requests(org_id,request_id,store_name,reason,input_hash) VALUES(7,?,?,?,?)", []byte(r.RequestID), []byte(r.Store), r.Reason, hash[:]).Error; err != nil {
		t.Fatal(err)
	}
	reason := "not_found"
	if authorized {
		reason = ""
	}
	if err = db.Exec("INSERT INTO qs_rm_replay_items(org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason) VALUES(7,?,0,?,3,?,?)", []byte(r.RequestID), []byte(eventID), authorized, reason).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSQLMongoCrossStoreNativeTwoOriginalTypesAndNoLookupSQL(t *testing.T) {
	for _, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			var evt event.DomainEvent
			if kind == "interpretation.report.generated" {
				payload, _ := mongoLocalGeneratedFixture(t, db)
				evt = mongoBatchNativeEvent(t, payload, kind, "cross-original-generated")
			} else {
				row := mongoBatchNativeAssessmentSheet()
				insertMongoLocalSheet(t, db, row)
				payload, err := mongoSubmissionPayload(row)
				if err != nil {
					t.Fatal(err)
				}
				evt = mongoBatchNativeEvent(t, payload, kind, "cross-original-sheet")
			}
			mongoBatchNativeIndexes(t, db)
			sources := mongoBatchNativeSources(t, evt)
			crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "closed", nil)
			if err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
				page, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, c, s, m, sources)
				if err != nil {
					return err
				}
				tx, _ := hostmysql.RequireTx(ctx)
				var before, after uint64
				var metric string
				if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &before); err != nil {
					return err
				}
				for i := 0; i < 10000; i++ {
					view, e := page.ResolveSource(ctx, sources[0])
					if e != nil {
						return e
					}
					if len(view.Observations) != 1 || len(view.OwnerBoundObservationKeys) != 1 || len(view.BlockingReasons) != 0 || !view.CurrentSQLCoverageObserved || !view.GlobalUnboundSQLCoverageRequired || !view.ExternalOriginAuthenticationRequired || !view.AIInboxCoverageRequired || view.DropReady {
						t.Logf("observation_count=%d owner_bound_count=%d block_categories=%v", len(view.Observations), len(view.OwnerBoundObservationKeys), view.BlockingReasons)
						return ErrSQLMongoCrossStoreConflict
					}
					if kind == "answersheet.submitted" && !containsString(view.Gaps, "storage_precision_gap") {
						return ErrSQLMongoCrossStoreConflict
					}
					view.OwnerBoundObservationKeys[0] = "edited"
					view.Observations[0].Inner.ID = "edited"
				}
				if err = tx.Raw("SHOW SESSION STATUS LIKE 'Com_select'").Row().Scan(&metric, &after); err != nil {
					return err
				}
				if after != before {
					return ErrSQLMongoCrossStoreConflict
				}
				t.Logf("mongo_original_type=%s cross_store_cached_lookups=10000 new_sql_selects=%d exact_fullrow_queries=%d drop_ready=false", kind, after-before, page.Report().Queries)
				if _, e := json.Marshal(page); !errors.Is(e, ErrSourceSerialization) {
					return ErrSQLMongoCrossStoreConflict
				}
				return page.ValidateBorrowedSnapshot(ctx)
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSQLMongoCrossStoreNativeHeldDeadLetterAndGovernanceResponsibility(t *testing.T) {
	for _, name := range []string{"held_pending", "deadletter_manual", "deadletter_transport_terminal", "deadletter_resolved", "governance_pending", "unrelated_governance_pending", "wire_time_conflict", "cross_org", "same_event_different_bytes", "source_type_impersonation"} {
		t.Run(name, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			mongoBatchNativeIndexes(t, db)
			row := mongoBatchNativeAssessmentSheet()
			insertMongoLocalSheet(t, db, row)
			payload, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			evt := mongoBatchNativeEvent(t, payload, "answersheet.submitted", "cross-source-negative")
			sources := mongoBatchNativeSources(t, evt)
			var change func([]byte) []byte
			if name == "wire_time_conflict" || name == "same_event_different_bytes" {
				change = func(raw []byte) []byte {
					var v originalDomainEnvelope
					if json.Unmarshal(raw, &v) != nil {
						t.Fatal("fixture")
					}
					v.OccurredAt = v.OccurredAt.Add(time.Millisecond)
					out, e := json.Marshal(v)
					if e != nil {
						t.Fatal(e)
					}
					return out
				}
			}
			if name == "source_type_impersonation" {
				change = func(raw []byte) []byte {
					var v originalDomainEnvelope
					if json.Unmarshal(raw, &v) != nil {
						t.Fatal("fixture")
					}
					v.EventType = "evaluation.requested"
					v.AggregateType = "Evaluation"
					v.AggregateID = "42"
					out, e := json.Marshal(v)
					if e != nil {
						t.Fatal(e)
					}
					return out
				}
			}
			switch name {
			case "deadletter_manual":
				crossMongoNativeWire(t, sqlDB, evt, "dead", "manual_required", "a", nil)
			case "deadletter_transport_terminal":
				crossMongoNativeWire(t, sqlDB, evt, "dead", "terminal", "a", nil)
			case "deadletter_resolved":
				crossMongoNativeWire(t, sqlDB, evt, "dead", "resolved_verified", "a", nil)
			case "governance_pending", "unrelated_governance_pending":
				action := "events.replay_delivery"
				if name == "unrelated_governance_pending" {
					action = "cache.manual_warmup"
				}
				if err := sqlDB.Exec("INSERT INTO system_governance_action_runs(request_id,action_id,org_id,input_json,status,started_at) VALUES('native-action',?,7,'{}','pending_reconciliation',UTC_TIMESTAMP(3))", action).Error; err != nil {
					t.Fatal(err)
				}
			default:
				state := "replayed"
				if name == "held_pending" {
					state = "blocked"
				}
				crossMongoNativeWire(t, sqlDB, evt, "held", state, "a", change)
			}
			if name == "cross_org" {
				if err := sqlDB.Exec("UPDATE retry_event_hold SET org_id=8").Error; err != nil {
					t.Fatal(err)
				}
			}
			if name == "same_event_different_bytes" {
				crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "b", nil)
			}
			err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
				page, e := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, c, s, m, sources)
				conflict := name == "wire_time_conflict" || name == "cross_org" || name == "same_event_different_bytes" || name == "source_type_impersonation"
				if conflict {
					if e == nil {
						return ErrSQLMongoCrossStore
					}
					return nil
				}
				if e != nil {
					return e
				}
				v, e := page.ResolveSource(ctx, sources[0])
				if e != nil {
					return e
				}
				wantClosed := name == "deadletter_resolved" || name == "unrelated_governance_pending"
				if (len(v.BlockingReasons) == 0) != wantClosed || len(v.Observations) == 0 {
					return ErrSQLMongoCrossStoreConflict
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSQLMongoCrossStoreNativeReplayOriginalAuthorization(t *testing.T) {
	for _, name := range []string{"denied_original", "authorized_without_actual_current", "wrong_namespace", "wrong_input_fingerprint", "authorized_current_pending", "authorized_current_orphan"} {
		t.Run(name, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			mongoBatchNativeIndexes(t, db)
			row := mongoBatchNativeAssessmentSheet()
			if name == "authorized_current_pending" {
				// A real current SDK event requires the native atomic owner
				// marker. This test explicitly creates that current contract;
				// the read-only retirement adapter never adds such a marker.
				row.DurableAcceptance = &sheetmongo.DurableAcceptancePO{SchemaVersion: 1, EventID: "cross-source-replay", AcceptedAt: row.FilledAt}
			}
			insertMongoLocalSheet(t, db, row)
			payload, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			evt := mongoBatchNativeEvent(t, payload, "answersheet.submitted", "cross-source-replay")
			sources := mongoBatchNativeSources(t, evt)
			store := "mongo-domain-events"
			if name == "wrong_namespace" {
				store = "assessment-mysql-outbox"
			}
			authorized := name == "authorized_without_actual_current" || name == "authorized_current_pending" || name == "authorized_current_orphan"
			crossMongoNativeReplay(t, sqlDB, evt.EventID(), store, authorized, name == "wrong_input_fingerprint")
			if name == "authorized_current_pending" || name == "authorized_current_orphan" {
				if _, err := db.Collection("rm_outbox").InsertOne(t.Context(), mongoCycleNativeMessage(t, evt, "pending")); err != nil {
					t.Fatal(err)
				}
			}
			if err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
				page, e := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, c, s, m, sources)
				if name == "wrong_namespace" || name == "wrong_input_fingerprint" || name == "authorized_current_orphan" {
					if e == nil {
						return ErrSQLMongoCrossStore
					}
					return nil
				}
				if e != nil {
					return e
				}
				view, e := page.ResolveSource(ctx, sources[0])
				if e != nil {
					return e
				}
				if len(view.Observations) != 1 || (len(view.BlockingReasons) == 0) != (name == "denied_original") {
					t.Logf("replay observation_count=%d block_categories=%v", len(view.Observations), view.BlockingReasons)
					return ErrSQLMongoCrossStoreConflict
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSQLMongoCrossStoreNativeProvisionalResolutionPreservesOtherCauses(t *testing.T) {
	for _, name := range []string{"valid_closed", "same_original_pending", "same_original_lease", "second_unknown_owner", "second_real_invalid", "same_id_other_type", "same_id_other_org", "source_content_conflict", "matching_owner_wire_digest_conflict"} {
		t.Run(name, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoCycleNativeDB(t)
			payload, _ := mongoLocalGeneratedFixture(t, db)
			if name == "source_content_conflict" {
				payload.GeneratedAt = payload.GeneratedAt.Add(time.Millisecond)
			}
			evt := mongoBatchNativeEvent(t, payload, "interpretation.report.generated", "cross-provisional-original")
			sources := mongoBatchNativeSources(t, evt)
			mongoBatchNativeIndexes(t, db)
			state := "replayed"
			if name == "same_original_pending" {
				state = "blocked"
			}
			var change func([]byte) []byte
			if name == "matching_owner_wire_digest_conflict" {
				change = func(raw []byte) []byte {
					var inner originalDomainEnvelope
					if json.Unmarshal(raw, &inner) != nil {
						t.Fatal("fixture")
					}
					inner.OccurredAt = inner.OccurredAt.Add(time.Millisecond)
					out, err := json.Marshal(inner)
					if err != nil {
						t.Fatal(err)
					}
					return out
				}
			}
			crossMongoNativeWire(t, sqlDB, evt, "held", state, "original", change)
			if name == "same_original_lease" {
				if err := sqlDB.Exec("UPDATE retry_event_hold SET claim_token='actual-held-token',claim_expires_at=UTC_TIMESTAMP(3)+INTERVAL 1 MINUTE").Error; err != nil {
					t.Fatal(err)
				}
			}
			if name == "second_unknown_owner" || name == "second_real_invalid" || name == "same_id_other_type" || name == "same_id_other_org" {
				second := mongoBatchNativeEvent(t, payload, "interpretation.report.generated", "cross-provisional-another")
				var edit func([]byte) []byte
				if name != "second_unknown_owner" {
					second = evt
					edit = func(raw []byte) []byte {
						var inner originalDomainEnvelope
						if json.Unmarshal(raw, &inner) != nil {
							t.Fatal("fixture")
						}
						switch name {
						case "same_id_other_type":
							inner.EventType = "evaluation.requested"
						case "same_id_other_org":
							var body map[string]json.RawMessage
							if json.Unmarshal(inner.Data, &body) != nil {
								t.Fatal("fixture")
							}
							body["org_id"] = json.RawMessage("8")
							inner.Data, _ = json.Marshal(body)
						default:
							inner.Data = json.RawMessage(`{"unknown_future_field":true}`)
						}
						out, err := json.Marshal(inner)
						if err != nil {
							t.Fatal(err)
						}
						return out
					}
				}
				crossMongoNativeWire(t, sqlDB, second, "held", "replayed", "other", edit)
			}
			wantError := name == "second_real_invalid" || name == "same_id_other_type" || name == "same_id_other_org" || name == "source_content_conflict" || name == "matching_owner_wire_digest_conflict"
			err := crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
				p, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, c, s, m, sources)
				if wantError {
					if err == nil {
						return ErrSQLMongoCrossStoreConflict
					}
					return nil
				}
				if err != nil {
					return err
				}
				view, err := p.ResolveSource(ctx, sources[0])
				if err != nil {
					return err
				}
				if (len(view.BlockingReasons) == 0) != (name == "valid_closed") || view.DropReady || (len(view.ProvisionalOwnerResolvedObservationKeys) == 1) != (name == "valid_closed" || name == "second_unknown_owner") {
					t.Logf("provisional_count=%d bound_count=%d block_categories=%v", len(view.ProvisionalOwnerResolvedObservationKeys), len(view.OwnerBoundObservationKeys), view.BlockingReasons)
					return ErrSQLMongoCrossStoreConflict
				}
				if name == "second_unknown_owner" && (len(view.Observations) != 2 || len(view.OwnerBoundObservationKeys) != 1) {
					return ErrSQLMongoCrossStoreConflict
				}
				return nil
			})
			// A source conflicting with the original Mongo graph can be
			// rejected by its opaque batch before cross-store preparation.
			if wantError && err != nil && name == "source_content_conflict" {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSQLMongoCrossStoreNativeGenuineNewSnapshotRequiredAndRawMutation(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoCycleNativeDB(t)
	mongoBatchNativeIndexes(t, db)
	row := mongoBatchNativeAssessmentSheet()
	insertMongoLocalSheet(t, db, row)
	payload, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	evt := mongoBatchNativeEvent(t, payload, "answersheet.submitted", "cross-source-fresh")
	sources := mongoBatchNativeSources(t, evt)
	crossMongoNativeWire(t, sqlDB, evt, "held", "replayed", "a", nil)
	var original *SQLMongoCrossStoreResponsibilityPage
	if err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
		var e error
		original, e = PrepareSQLMongoCrossStoreResponsibilityPage(ctx, c, s, m, sources)
		if e != nil {
			return e
		}
		if original.RecheckBusiness(ctx, c, s, m) == nil {
			return ErrSQLMongoCrossStore
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
		return original.RecheckBusiness(ctx, c, s, m)
	}); err != nil {
		t.Fatal("actual new paired snapshot rejected", err)
	}
	if err := sqlDB.Exec("UPDATE retry_event_hold SET last_error='non-content concurrent metadata mutation'").Error; err != nil {
		t.Fatal(err)
	}
	err = crossMongoNativePair(t, sqlDB, client, db, config, sources, func(ctx context.Context, c *SQLCrossStoreResponsibilityCatalog, s *SQLBusinessOwnerBatch, m *MongoHistoricalOwnerBatch) error {
		return original.RecheckBusiness(ctx, c, s, m)
	})
	if !errors.Is(err, ErrSQLMongoCrossStoreConflict) {
		t.Fatal("all-column SQL row change not detected", err)
	}
	if err = original.ValidateBorrowedSnapshot(t.Context()); err == nil {
		t.Fatal("completed actual host transaction was accepted")
	}
	var after bson.M
	if err = db.Collection("answersheets").FindOne(t.Context(), bson.M{"domain_id": int64(10042)}).Decode(&after); err != nil {
		t.Fatal(err)
	}
	if _, present := after["durable_acceptance"]; present {
		t.Fatal("read-only adapter invented acceptance")
	}
}
