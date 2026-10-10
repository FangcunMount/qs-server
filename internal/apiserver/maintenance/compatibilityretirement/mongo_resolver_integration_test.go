//go:build integration

package retirement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mongoLocalNativeDB(t *testing.T) (*mongo.Client, *mongo.Database, MongoOwnerConfig) {
	t.Helper()
	client, db := mongodbtest.ReplicaSetDatabase(t)
	for _, name := range []string{"schema_migrations", "answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "rm_outbox", "qs_rm_replay_requests"} {
		if err := db.CreateCollection(t.Context(), name, options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}); err != nil {
		t.Fatal(err)
	}
	// Independent inventory-compatible framing, not observe(...).identity.
	var hello bson.M
	if err := client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatal(err)
	}
	stable := bson.D{}
	for _, key := range []string{"setName", "hosts", "me"} {
		if value, ok := hello[key]; ok {
			stable = append(stable, bson.E{Key: key, Value: value})
		}
	}
	encoded, err := json.Marshal(stable)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := db.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Info struct {
			UUID primitive.Binary `bson:"uuid"`
		} `bson:"info"`
	}
	if err = cursor.All(t.Context(), &rows); err != nil || len(rows) != 1 {
		t.Fatal("actual migration UUID", err)
	}
	h := sha256.New()
	for _, part := range []string{"mongodb_database_identity_v1", string(encoded), db.Name(), hex.EncodeToString(rows[0].Info.UUID.Data)} {
		frame := make([]byte, 9)
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:], uint64(len(part)))
		if _, err = h.Write(append(frame, []byte(part)...)); err != nil {
			t.Fatal(err)
		}
	}
	return client, db, MongoOwnerConfig{ExpectedIdentityHash: hex.EncodeToString(h.Sum(nil)), ExpectedMigrationVersion: 38}
}

func mongoLocalNativeTx(t *testing.T, client *mongo.Client, fn func(mongo.SessionContext) error) error {
	t.Helper()
	session, err := client.StartSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.EndSession(t.Context())
	_, err = session.WithTransaction(t.Context(), func(ctx mongo.SessionContext) (any, error) { return nil, fn(ctx) })
	return err
}

func mongoLocalNativeSource(t *testing.T, p any, eventType, aggregate, id string) *DecodedSourceEvent {
	t.Helper()
	b := event.BaseEvent{ID: id, EventTypeValue: eventType, AggregateTypeValue: aggregate, OccurredAtValue: time.Date(2026, 10, 8, 1, 2, 4, 456789123, time.UTC)}
	var evt event.DomainEvent
	switch value := p.(type) {
	case eventpayload.AnswerSheetSubmittedData:
		b.AggregateIDValue = value.AnswerSheetID
		evt = event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: b, Data: value}
	case eventoutcome.ReportGeneratedPayload:
		b.AggregateIDValue = value.GenerationID
		evt = event.Event[eventoutcome.ReportGeneratedPayload]{BaseEvent: b, Data: value}
	default:
		t.Fatal("fixture type unsupported")
	}
	raw, err := domainwire.EncodeEvent(evt)
	if err != nil {
		t.Fatal(err)
	}
	return readOneMongo(t, setMongoField(fixtureMongoRow(t, raw, primitive.NewObjectID()), "org_id", int64(7)))
}

func insertMongoLocalSheet(t *testing.T, db *mongo.Database, row sheetmongo.AnswerSheetPO) {
	t.Helper()
	if _, err := db.Collection("answersheets").InsertOne(t.Context(), row); err != nil {
		t.Fatal(err)
	}
}

func mongoLocalStandardFixture(t *testing.T, p eventpayload.AnswerSheetSubmittedData, id, state string) bson.D {
	t.Helper()
	cfg, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evt := event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: event.BaseEvent{ID: id, EventTypeValue: "answersheet.submitted", AggregateTypeValue: "AnswerSheet", AggregateIDValue: p.AnswerSheetID, OccurredAtValue: p.SubmittedAt.Add(time.Second)}, Data: p}
	intents, err := standard.PrepareIntents([]event.DomainEvent{evt}, eventcatalog.NewCatalog(cfg), "api-server")
	if err != nil || len(intents) != 1 {
		t.Fatal("real standard producer", err)
	}
	in := intents[0].Message.Input()
	fp := intents[0].Message.Fingerprint()
	return bson.D{{Key: "_id", Value: bson.D{{Key: "producer", Value: in.Producer}, {Key: "message_id", Value: in.ID}, {Key: "destination", Value: in.Destination}}}, {Key: "producer", Value: in.Producer}, {Key: "message_id", Value: in.ID}, {Key: "destination", Value: in.Destination}, {Key: "event_type", Value: in.EventType}, {Key: "schema_version", Value: in.SchemaVersion}, {Key: "scope", Value: in.Scope}, {Key: "content_type", Value: in.ContentType}, {Key: "occurred_at", Value: in.OccurredAt}, {Key: "payload", Value: in.Payload}, {Key: "fingerprint", Value: fp[:]}, {Key: "state", Value: state}, {Key: "version", Value: int64(3)}, {Key: "transport_confirmed_at", Value: p.SubmittedAt.Add(2 * time.Second)}, {Key: "failure_count", Value: int64(1)}}
}

func TestMongoLocalNativeIndependentBorrowedTransactionAndRecheck(t *testing.T) {
	client, db, config := mongoLocalNativeDB(t)
	row := mongoLocalSheet()
	insertMongoLocalSheet(t, db, row)
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-submission")
	if _, err := ResolveMongoOwner(t.Context(), db, config, source, nil); err == nil {
		t.Fatal("normal context bypassed borrowed transaction")
	}
	var observed *MongoOwnerResolution
	rollback := errors.New("host rollback")
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var e error
		observed, e = ResolveMongoOwner(ctx, db, config, source, nil)
		if e != nil {
			return e
		}
		local := observed.Local()
		if !local.OwnerLocalTerminal || local.FrozenAdmissionPurpose != "independent_questionnaire" || !local.SourceAuthenticationRequired || !local.SQLResponsibilityRequired || !local.GlobalUnboundResponsibilityCoverageRequired {
			t.Fatal("local qualification claims full closure")
		}
		local.BlockingReasons = append(local.BlockingReasons, "edited")
		if len(observed.Local().BlockingReasons) != 0 {
			t.Fatal("public local result aliases baseline")
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var after sheetmongo.AnswerSheetPO
	if db.Collection("answersheets").FindOne(t.Context(), bson.M{"domain_id": int64(10042)}).Decode(&after) != nil || after.DurableAcceptance != nil || after.LegacySubmissionEvidence != nil {
		t.Fatal("read-only resolver invented acceptance/history")
	}
	source.Submitted.Admission.QuestionnaireCode = "edited"
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error { return observed.RecheckMongo(ctx) }); err != nil {
		t.Fatal("editable source changed sealed baseline", err)
	}
	if _, err = db.Collection("answersheets").UpdateOne(t.Context(), bson.M{"domain_id": int64(10042)}, bson.M{"$set": bson.M{"questionnaire_title": "concurrent mutation"}}); err != nil {
		t.Fatal(err)
	}
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error { return observed.RecheckMongo(ctx) }); !errors.Is(err, ErrMongoOwnerConflict) {
		t.Fatal("full BSON mutation ignored", err)
	}
}

func TestMongoLocalNativeNamespaceUUIDAndBorrowedClientRejections(t *testing.T) {
	client, db, config := mongoLocalNativeDB(t)
	row := mongoLocalSheet()
	insertMongoLocalSheet(t, db, row)
	p, _ := mongoSubmissionPayload(row)
	source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-submission")
	var observed *MongoOwnerResolution
	if err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		var e error
		observed, e = ResolveMongoOwner(ctx, db, config, source, nil)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	otherClient, otherDB, otherConfig := mongoLocalNativeDB(t)
	if err := mongoLocalNativeTx(t, otherClient, func(ctx mongo.SessionContext) error {
		_, e := ResolveMongoOwner(ctx, db, config, source, nil)
		return e
	}); err == nil {
		t.Fatal("other borrowed client's session accepted")
	}
	if err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := ResolveMongoOwner(ctx, otherDB, otherConfig, source, nil)
		return e
	}); err == nil {
		t.Fatal("original session silently rebound another client")
	}
	var original bson.Raw
	if err := db.Collection("answersheets").FindOne(t.Context(), bson.M{"domain_id": int64(10042)}).Decode(&original); err != nil {
		t.Fatal(err)
	}
	if err := db.Collection("answersheets").Drop(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCollection(t.Context(), "answersheets", options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("answersheets").InsertOne(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	if err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error { return observed.RecheckMongo(ctx) }); !errors.Is(err, ErrMongoOwnerConflict) {
		t.Fatal("same IDs and exact BSON reused under a new namespace UUID", err)
	}
}

func TestMongoLocalNativeIdentityUnknownAndAdmissionRejections(t *testing.T) {
	for _, kind := range []string{"wrong_identity", "dirty", "head", "missing_admission", "cross_org", "duplicate_owner", "unknown_field", "double_id", "casefold_collision", "assessment_missing_sql"} {
		t.Run(kind, func(t *testing.T) {
			client, db, config := mongoLocalNativeDB(t)
			row := mongoLocalSheet()
			p, _ := mongoSubmissionPayload(row)
			source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-submission")
			switch kind {
			case "wrong_identity":
				config.ExpectedIdentityHash = strings.Repeat("a", 64)
			case "dirty":
				if _, err := db.Collection("schema_migrations").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"dirty": true}}); err != nil {
					t.Fatal(err)
				}
			case "head":
				config.ExpectedMigrationVersion = 37
			case "missing_admission":
				row.Admission = nil
			case "cross_org":
				row.OrgID = 8
			case "double_id":
				raw, _ := bson.Marshal(row)
				var d bson.D
				if bson.Unmarshal(raw, &d) != nil {
					t.Fatal("fixture")
				}
				d = setMongoField(d, "domain_id", float64(10042))
				if _, err := db.Collection("answersheets").InsertOne(t.Context(), d); err != nil {
					t.Fatal(err)
				}
			case "unknown_field":
				raw, _ := bson.Marshal(row)
				var d bson.D
				if bson.Unmarshal(raw, &d) != nil {
					t.Fatal("fixture")
				}
				d = append(d, bson.E{Key: "unknown_semantics", Value: true})
				if _, err := db.Collection("answersheets").InsertOne(t.Context(), d); err != nil {
					t.Fatal(err)
				}
			case "casefold_collision":
				row.Admission.QuestionnaireCode = "q"
				row.QuestionnaireCode = "q"
			case "assessment_missing_sql":
				row.Admission.Purpose = "assessment"
				row.Admission.ModelKind = "scale"
				row.Admission.ModelCode = "M"
				row.Admission.ModelVersion = "1.0"
				p, _ = mongoSubmissionPayload(row)
				source = mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-submission")
			}
			if kind != "unknown_field" && kind != "double_id" {
				insertMongoLocalSheet(t, db, row)
			}
			if kind == "duplicate_owner" {
				copy := row
				copy.ID = primitive.NewObjectID()
				insertMongoLocalSheet(t, db, copy)
			}
			if err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
				_, err := ResolveMongoOwner(ctx, db, config, source, nil)
				return err
			}); err == nil {
				t.Fatal("unproven owner or identity accepted")
			}
		})
	}
}

func TestMongoLocalNativeCurrentOutboxAndReplayResponsibilities(t *testing.T) {
	for _, kind := range []string{"published_closed", "pending", "unknown_state", "lease", "orphan_markerless", "fingerprint", "wrong_org", "replay_authorized_unknown", "replay_authorized_closed", "replay_denied", "replay_hash_conflict", "org_scan_cap", "published_confirmation_absent", "missing_replay_boolean", "replay_wrong_store"} {
		t.Run(kind, func(t *testing.T) {
			client, db, config := mongoLocalNativeDB(t)
			row := mongoLocalSheet()
			p, _ := mongoSubmissionPayload(row)
			source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-submission")
			row.DurableAcceptance = &sheetmongo.DurableAcceptancePO{SchemaVersion: 1, EventID: source.EventID, AcceptedAt: row.FilledAt}
			if kind == "orphan_markerless" {
				row.DurableAcceptance = nil
			}
			insertMongoLocalSheet(t, db, row)
			doc := mongoLocalStandardFixture(t, p, source.EventID, "published")
			switch kind {
			case "pending":
				doc = setMongoField(doc, "state", "pending")
			case "unknown_state":
				doc = setMongoField(doc, "state", "unknown")
			case "lease":
				doc = append(doc, bson.E{Key: "lease_until", Value: time.Now()})
			case "fingerprint":
				doc = setMongoField(doc, "fingerprint", make([]byte, 32))
			case "wrong_org":
				doc = setMongoField(doc, "scope", "org:8")
			case "published_confirmation_absent":
				doc = omitMongoField(doc, "transport_confirmed_at")
			case "replay_authorized_closed":
				doc = append(doc, bson.E{Key: "manual_replay_request_id", Value: "request-1"}, bson.E{Key: "manual_replay_version", Value: int64(1)})
			}
			if _, err := db.Collection("rm_outbox").InsertOne(t.Context(), doc); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(kind, "replay_") {
				request := standard.ReplayRequest{OrgID: 7, RequestID: "request-1", Store: "mongo-domain-events", Reason: "native fixture", Targets: []standard.ReplayTarget{{EventID: source.EventID, ExpectedFailureCount: 1}}}
				if kind == "replay_wrong_store" {
					request.Store = "mysql-evaluation-outcome"
				}
				hash, err := request.Fingerprint()
				if err != nil {
					t.Fatal(err)
				}
				if kind == "replay_hash_conflict" {
					hash[0] ^= 1
				}
				if _, err = db.Collection("qs_rm_replay_requests").InsertOne(t.Context(), bson.M{"_id": "7:request-1", "org_id": int64(7), "request_id": request.RequestID, "store_name": request.Store, "reason": request.Reason, "input_hash": hash[:], "items": bson.A{bson.M{"event_id": source.EventID, "expected_failure_count": int64(1), "authorized": kind != "replay_denied", "reason": "native_result"}}}); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "missing_replay_boolean" {
				request := standard.ReplayRequest{OrgID: 7, RequestID: "request-1", Store: "mongo-domain-events", Reason: "native fixture", Targets: []standard.ReplayTarget{{EventID: source.EventID, ExpectedFailureCount: 1}}}
				hash, e := request.Fingerprint()
				if e != nil {
					t.Fatal(e)
				}
				if _, e = db.Collection("qs_rm_replay_requests").InsertOne(t.Context(), bson.M{"_id": "7:request-1", "org_id": int64(7), "request_id": "request-1", "store_name": "mongo-domain-events", "reason": request.Reason, "input_hash": hash[:], "items": bson.A{bson.M{"event_id": source.EventID, "expected_failure_count": int64(1), "reason": "native_result"}}}); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "org_scan_cap" {
				for n := 0; n < MongoOwnerRowLimit; n++ {
					other := mongoLocalStandardFixture(t, p, fmt.Sprintf("other-%d", n), "published")
					if _, err := db.Collection("rm_outbox").InsertOne(t.Context(), other); err != nil {
						t.Fatal(err)
					}
				}
			}
			var local MongoLocalResolution
			err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
				r, e := ResolveMongoOwner(ctx, db, config, source, nil)
				if e == nil {
					local = r.Local()
				}
				return e
			})
			switch kind {
			case "published_closed", "replay_denied", "replay_authorized_closed":
				if err != nil || !local.OwnerLocalTerminal || len(local.BlockingReasons) != 0 {
					t.Fatal("terminal local case rejected", err)
				}
			case "pending", "lease", "orphan_markerless", "replay_authorized_unknown":
				if err != nil || local.OwnerLocalTerminal || len(local.BlockingReasons) == 0 {
					t.Fatal("unfinished current responsibility accepted", err)
				}
			default:
				if err == nil {
					t.Fatal("unknown/corrupt/incomplete current rows accepted")
				}
			}
		})
	}
}

// This validation authorizes only the CI test fixture's schema lifecycle;
// it provides no retirement, production ownership or deletion capability.
func validHistoricalCIMySQLID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == id
}

func historicalCIMySQLMatches(raw []byte, containerID string) bool {
	var rows []struct {
		ID     string `json:"Id"`
		Config struct{ Image string }
		State  struct {
			Running bool
			Health  struct{ Status string }
		}
		HostConfig struct {
			PortBindings map[string][]struct{ HostIP, HostPort string }
		}
		NetworkSettings struct {
			Ports map[string][]struct{ HostIP, HostPort string }
		}
	}
	if !validHistoricalCIMySQLID(containerID) || json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || rows[0].ID != containerID || rows[0].Config.Image != "mysql:8.4" || !rows[0].State.Running || rows[0].State.Health.Status != "healthy" {
		return false
	}
	requested := rows[0].HostConfig.PortBindings["3306/tcp"]
	actual := rows[0].NetworkSettings.Ports["3306/tcp"]
	if len(requested) != 1 || requested[0].HostPort != "3306" || (requested[0].HostIP != "" && requested[0].HostIP != "0.0.0.0") || len(actual) < 1 || len(actual) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, binding := range actual {
		if binding.HostPort != "3306" || (binding.HostIP != "0.0.0.0" && binding.HostIP != "::") || seen[binding.HostIP] {
			return false
		}
		seen[binding.HostIP] = true
	}
	return seen["0.0.0.0"]
}

func TestHistoricalCIMySQLFixtureRequiresActualJobService(t *testing.T) {
	id := strings.Repeat("a", 64)
	valid := `[{"Id":"` + id + `","Config":{"Image":"mysql:8.4"},"State":{"Running":true,"Health":{"Status":"healthy"}},"HostConfig":{"PortBindings":{"3306/tcp":[{"HostIp":"","HostPort":"3306"}]}},"NetworkSettings":{"Ports":{"3306/tcp":[{"HostIp":"0.0.0.0","HostPort":"3306"},{"HostIp":"::","HostPort":"3306"}]}}}]`
	if !historicalCIMySQLMatches([]byte(valid), id) {
		t.Fatal("actual job service rejected")
	}
	for name, raw := range map[string]string{
		"wrong_container":      strings.Replace(valid, `"Id":"`+id, `"Id":"`+strings.Repeat("b", 64), 1),
		"wrong_image":          strings.Replace(valid, "mysql:8.4", "mysql:8.0", 1),
		"stopped":              strings.Replace(valid, `"Running":true`, `"Running":false`, 1),
		"unhealthy":            strings.Replace(valid, "healthy", "starting", 1),
		"wrong_requested_port": strings.Replace(valid, `"HostPort":"3306"`, `"HostPort":"34306"`, 1),
		"wrong_actual_port":    strings.Replace(valid, `"HostIp":"0.0.0.0","HostPort":"3306"`, `"HostIp":"0.0.0.0","HostPort":"34306"`, 1),
		"other_host":           strings.Replace(valid, `"HostIp":"0.0.0.0"`, `"HostIp":"192.0.2.1"`, 1),
		"duplicate_binding":    strings.Replace(valid, `"HostIp":"::"`, `"HostIp":"0.0.0.0"`, 1),
		"duplicate_container":  strings.TrimSuffix(valid, "]") + "," + strings.TrimPrefix(valid, "["),
		"malformed":            "{",
	} {
		t.Run(name, func(t *testing.T) {
			if historicalCIMySQLMatches([]byte(raw), id) {
				t.Fatal("unbound CI service accepted")
			}
		})
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 64), strings.Repeat("g", 64)} {
		if historicalCIMySQLMatches([]byte(valid), bad) {
			t.Fatal("invalid requested container ID accepted")
		}
	}
}

func mongoLocalSQLFixture(t *testing.T) *gorm.DB {
	t.Helper()
	cfg, err := drivermysql.ParseDSN(os.Getenv("QS_HISTORY_MYSQL_DSN"))
	if err != nil || cfg.Net != "tcp" {
		t.Fatal("explicit owned loopback SQL fixture required")
	}
	ciContainer := os.Getenv("QS_HISTORY_CI_MYSQL_CONTAINER")
	if ciContainer == "" {
		if cfg.Addr != "127.0.0.1:34306" {
			t.Fatal("explicit root-owned loopback SQL fixture required")
		}
	} else {
		// CI uses the already-owned job service, not a second fixed host port.
		// The caller supplies its actual container ID; inspect must prove the
		// running service and fixed route before any schema may be created.
		if !validHistoricalCIMySQLID(ciContainer) || cfg.Addr != "127.0.0.1:3306" {
			t.Fatal("actual CI MySQL service binding required")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		raw, inspectErr := exec.CommandContext(ctx, "docker", "--host", "unix:///var/run/docker.sock", "inspect", ciContainer).Output()
		if inspectErr != nil || !historicalCIMySQLMatches(raw, ciContainer) {
			t.Fatal("actual CI MySQL service inspection rejected")
		}
	}
	cfg.DBName = ""
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("qs_mongo_owner_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error("own schema cleanup failed")
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	})
	migrateHistoricalA99Fixture(t, pool, name)
	at := mongoLocalSheet().FilledAt
	if err = db.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_algorithm,evaluation_model_code,evaluation_model_version,created_at,updated_at,submitted_at,evaluated_at,version) VALUES(42,7,21,'Q','1.0',10042,'adhoc','evaluated','scale',?,'M','1.0',?,?,?,?,1)", string(modelcatalog.AlgorithmScaleDefault), at, at, at, at).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at) VALUES('evaluation_run','42:1',1,42,'succeeded',?,?)", at, at).Error; err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{ID: meta.FromUint64(9001), OrgID: 7, AssessmentID: meta.FromUint64(42), TesteeID: 21, RunID: "42:1", Model: domainoutcome.ModelIdentity{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault, Code: "M", Version: "1.0", Title: "Model"}, Payload: []byte(`{"score":5}`), SchemaVersion: 2, EvaluatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlevaluation.NewOutcomeRepository(db).Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	return db
}

func mongoLocalSQLFacts(t *testing.T, db *gorm.DB, id, eventID string) *sqlevaluation.SQLHistoricalOwnerFacts {
	t.Helper()
	var observed *sqlevaluation.SQLHistoricalOwnerFacts
	if err := db.Transaction(func(tx *gorm.DB) error {
		var server, database string
		if err := tx.Raw("SELECT @@server_uuid,DATABASE()").Row().Scan(&server, &database); err != nil {
			return err
		}
		hash := mongoOwnerHashParts("mysql_database_identity_v1", server, database)
		var err error
		if id == "sheet" {
			observed, err = sqlevaluation.PrepareSQLHistoricalAnswerSheetFacts(hostmysql.WithTx(t.Context(), tx), 10042, eventID, hash)
		} else {
			observed, err = sqlevaluation.PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, eventID, hash)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return observed
}

func TestMongoLocalNativeAssessmentAdmissionActualUniqueSQLClosure(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	client, db, config := mongoLocalNativeDB(t)
	row := mongoLocalSheet()
	row.Admission.Purpose = "assessment"
	row.Admission.ModelKind = "scale"
	row.Admission.ModelAlgorithm = string(modelcatalog.AlgorithmScaleDefault)
	row.Admission.ModelCode = "M"
	row.Admission.ModelVersion = "1.0"
	row.Admission.ModelTitle = "Model"
	insertMongoLocalSheet(t, db, row)
	p, err := mongoSubmissionPayload(row)
	if err != nil {
		t.Fatal(err)
	}
	source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-assessment-submission")
	facts := mongoLocalSQLFacts(t, sqlDB, "sheet", source.EventID)
	general := mongoLocalSQLFacts(t, sqlDB, "assessment", source.EventID)
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := ResolveMongoOwner(ctx, db, config, source, general)
		return e
	}); err == nil {
		t.Fatal("Assessment ID point-read upgraded to a proved unique AnswerSheet association")
	}
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		r, e := ResolveMongoOwner(ctx, db, config, source, facts)
		if e != nil {
			return e
		}
		local := r.Local()
		if !local.OwnerLocalTerminal || local.AssessmentID != 42 || local.FrozenAdmissionPurpose != "assessment" || !local.SQLCrossClosureRequired || local.SQLSubmissionClock == nil || local.SQLSubmissionClock.Precision != 0 || local.SQLSubmissionClock.ExactBusinessMilliseconds || !containsString(local.Gaps, "storage_precision_gap") {
			t.Fatal("unique frozen original SQL association not bound")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Exec("UPDATE assessment SET submitted_at=DATE_ADD(submitted_at, INTERVAL 1 SECOND) WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	crossSecond := mongoLocalSQLFacts(t, sqlDB, "sheet", source.EventID)
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := ResolveMongoOwner(ctx, db, config, source, crossSecond)
		return e
	}); err == nil {
		t.Fatal("cross-second SQL time drift hidden by precision gap")
	}
	if err = sqlDB.Exec("UPDATE assessment SET submitted_at=? WHERE id=42", row.FilledAt).Error; err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Exec("UPDATE assessment SET evaluation_model_version='different' WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	changed := mongoLocalSQLFacts(t, sqlDB, "sheet", source.EventID)
	if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
		_, e := ResolveMongoOwner(ctx, db, config, source, changed)
		return e
	}); err == nil {
		t.Fatal("model conflict accepted")
	}
}

func TestMongoLocalNativeSQLCurrentTerminalCanonicalExecution(t *testing.T) {
	for _, kind := range []string{"evaluated_success", "outcome_failed_run", "unknown_run", "outcome_finished_mismatch", "evaluated_owner_clock_mismatch", "failed_no_outcome_closed", "failed_without_clock", "failed_with_outcome", "failed_ambiguous_clock"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB := mongoLocalSQLFixture(t)
			client, db, config := mongoLocalNativeDB(t)
			row := mongoLocalSheet()
			row.Admission.Purpose = "assessment"
			row.Admission.ModelKind = "scale"
			row.Admission.ModelAlgorithm = string(modelcatalog.AlgorithmScaleDefault)
			row.Admission.ModelCode = "M"
			row.Admission.ModelVersion = "1.0"
			row.Admission.ModelTitle = "Model"
			insertMongoLocalSheet(t, db, row)
			p, err := mongoSubmissionPayload(row)
			if err != nil {
				t.Fatal(err)
			}
			source := mongoLocalNativeSource(t, p, "answersheet.submitted", "AnswerSheet", "original-canonical-submission")
			exec := func(query string, args ...any) {
				if err := sqlDB.Exec(query, args...).Error; err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "outcome_failed_run":
				exec("UPDATE runtime_checkpoint SET status='failed',retry_disposition='terminal' WHERE assessment_id=42")
			case "unknown_run":
				exec("UPDATE runtime_checkpoint SET status='unsupported' WHERE assessment_id=42")
			case "outcome_finished_mismatch":
				exec("UPDATE runtime_checkpoint SET finished_at=DATE_ADD(finished_at, INTERVAL 1 SECOND) WHERE assessment_id=42")
			case "evaluated_owner_clock_mismatch":
				exec("UPDATE assessment SET evaluated_at=DATE_ADD(evaluated_at, INTERVAL 1 SECOND) WHERE id=42")
			case "failed_no_outcome_closed", "failed_without_clock", "failed_with_outcome", "failed_ambiguous_clock":
				exec("UPDATE assessment SET status='failed',failed_at=? WHERE id=42", row.FilledAt)
				exec("UPDATE runtime_checkpoint SET status='failed',retry_disposition='terminal' WHERE assessment_id=42")
				if kind != "failed_with_outcome" {
					exec("DELETE FROM evaluation_outcome WHERE assessment_id=42")
				}
				if kind == "failed_without_clock" {
					exec("UPDATE assessment SET failed_at=NULL WHERE id=42")
				}
				if kind == "failed_ambiguous_clock" {
					exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at,retry_disposition) VALUES('evaluation_run','42:2',2,42,'failed',?,?,'terminal')", row.FilledAt, row.FilledAt)
				}
			}
			facts := mongoLocalSQLFacts(t, sqlDB, "sheet", source.EventID)
			err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
				r, resolveErr := ResolveMongoOwner(ctx, db, config, source, facts)
				if resolveErr != nil {
					return resolveErr
				}
				local := r.Local()
				wantTerminal := kind == "evaluated_success" || kind == "failed_no_outcome_closed"
				if local.OwnerLocalTerminal != wantTerminal {
					t.Fatal("current owner terminal declaration did not match actual canonical execution")
				}
				if local.OriginalRun != nil || !local.SourceAuthenticationRequired || !local.SQLCrossClosureRequired || !local.GlobalUnboundResponsibilityCoverageRequired {
					t.Fatal("current terminal owner inferred an undeclared original Run or cleared external gates")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mongoLocalGeneratedFixture(t *testing.T, db *mongo.Database) (eventoutcome.ReportGeneratedPayload, interpretmongo.ReportGenerationPO) {
	t.Helper()
	if _, err := db.Collection("interpret_report_artifacts").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "generation_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uk_artifact_generation_id").SetCollation(&options.Collation{Locale: "simple"})}); err != nil {
		t.Fatal(err)
	}
	at := mongoLocalSheet().FilledAt
	finished := at
	g := interpretmongo.ReportGenerationPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(1)}, OutcomeID: 9001, ReportType: "scale", TemplateVersion: "v1", Status: "generated", LatestRunID: 11, ReportID: 20, Version: 7}
	run := interpretmongo.InterpretationRunPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(11)}, GenerationID: 1, Attempt: 1, Status: "succeeded", StartedAt: &finished, FinishedAt: &finished}
	a := interpretmongo.InterpretReportPO{BaseDocument: base.BaseDocument{ID: primitive.NewObjectID(), DomainID: meta.FromUint64(20)}, GenerationID: 1, OutcomeID: 9001, InterpretationRunID: 11, ReportType: "scale", TemplateVersion: "v1", GeneratedAt: at, OrgID: 7, AssessmentID: 42, TesteeID: 21, Model: &interpretmongo.ModelIdentityPO{Kind: "scale", Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "M", Version: "1.0", Title: "Model"}, BuilderIdentity: "builder:v1", ContentSchemaVersion: "v1"}
	for name, document := range map[string]any{"report_generations": g, "interpretation_runs": run, "interpret_report_artifacts": a} {
		if _, err := db.Collection(name).InsertOne(t.Context(), document); err != nil {
			t.Fatal(err)
		}
	}
	p := eventoutcome.ReportGeneratedPayload{OrgID: 7, GenerationID: "1", RunID: "11", ReportID: "20", AssessmentID: "42", OutcomeID: "9001", TesteeID: 21, Attempt: 1, ReportType: "scale", TemplateVersion: "v1", BuilderIdentity: "builder:v1", ContentSchemaVersion: "v1", Model: eventoutcome.ModelIdentity{Kind: "scale", Algorithm: string(modelcatalog.AlgorithmScaleDefault), Code: "M", Version: "1.0", Title: "Model"}, GeneratedAt: at}
	return p, g
}

func TestMongoLocalNativeOriginalReportGraphNotCurrentWinner(t *testing.T) {
	sqlDB := mongoLocalSQLFixture(t)
	for _, kind := range []string{"original", "missing_original_report", "current_winner_substitution", "wrong_original_run", "cross_org", "orphan_artifact", "orphan_run", "unfinished_generation", "pending_run", "retry_pending", "duplicate_attempt", "newer_winner_original_absent", "raw_mutation", "uniqueness_unproven", "related_generation_wrong_current_artifact", "related_artifact_orphan_run"} {
		t.Run(kind, func(t *testing.T) {
			client, db, config := mongoLocalNativeDB(t)
			p, _ := mongoLocalGeneratedFixture(t, db)
			source := mongoLocalNativeSource(t, p, "interpretation.report.generated", "ReportGeneration", "original-report-generated")
			facts := mongoLocalSQLFacts(t, sqlDB, "assessment", source.EventID)
			update := func(name string, id uint64, set bson.M) {
				if _, err := db.Collection(name).UpdateOne(t.Context(), bson.M{"domain_id": int64(id)}, bson.M{"$set": set}); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "related_generation_wrong_current_artifact", "related_artifact_orphan_run":
				var g interpretmongo.ReportGenerationPO
				var a interpretmongo.InterpretReportPO
				var run interpretmongo.InterpretationRunPO
				if db.Collection("report_generations").FindOne(t.Context(), bson.M{"domain_id": int64(1)}).Decode(&g) != nil || db.Collection("interpret_report_artifacts").FindOne(t.Context(), bson.M{"domain_id": int64(20)}).Decode(&a) != nil || db.Collection("interpretation_runs").FindOne(t.Context(), bson.M{"domain_id": int64(11)}).Decode(&run) != nil {
					t.Fatal("fixture")
				}
				g.ID = primitive.NewObjectID()
				g.DomainID = meta.FromUint64(2)
				g.LatestRunID = 13
				g.ReportID = 21
				run.ID = primitive.NewObjectID()
				run.DomainID = meta.FromUint64(13)
				run.GenerationID = 2
				a.ID = primitive.NewObjectID()
				a.DomainID = meta.FromUint64(21)
				a.GenerationID = 2
				a.InterpretationRunID = 13
				if kind == "related_generation_wrong_current_artifact" {
					g.ReportID = 20
				} else {
					a.InterpretationRunID = 99
				}
				for name, document := range map[string]any{"report_generations": g, "interpretation_runs": run, "interpret_report_artifacts": a} {
					if _, err := db.Collection(name).InsertOne(t.Context(), document); err != nil {
						t.Fatal(err)
					}
				}
			case "uniqueness_unproven":
				if _, err := db.Collection("interpret_report_artifacts").Indexes().DropOne(t.Context(), "uk_artifact_generation_id"); err != nil {
					t.Fatal(err)
				}
			case "missing_original_report":
				source.Generated.ReportID = "21"
			case "current_winner_substitution":
				source.Generated.ReportID = "21"
				source.Generated.RunID = "12"
			case "wrong_original_run":
				source.Generated.RunID = "12"
			case "cross_org":
				update("interpret_report_artifacts", 20, bson.M{"org_id": int64(8)})
			case "unfinished_generation":
				update("report_generations", 1, bson.M{"status": "generating"})
			case "pending_run":
				update("interpretation_runs", 11, bson.M{"status": "running"})
			case "retry_pending":
				update("interpretation_runs", 11, bson.M{"next_attempt_at": time.Now(), "retry_disposition": "automatic"})
			case "duplicate_attempt", "orphan_run":
				var r interpretmongo.InterpretationRunPO
				if db.Collection("interpretation_runs").FindOne(t.Context(), bson.M{"domain_id": int64(11)}).Decode(&r) != nil {
					t.Fatal("fixture")
				}
				r.ID = primitive.NewObjectID()
				r.DomainID = meta.FromUint64(12)
				if kind == "orphan_run" {
					r.GenerationID = 999
					source.Generated.RunID = "12"
				}
				if _, err := db.Collection("interpretation_runs").InsertOne(t.Context(), r); err != nil {
					t.Fatal(err)
				}
			case "orphan_artifact":
				var a interpretmongo.InterpretReportPO
				if db.Collection("interpret_report_artifacts").FindOne(t.Context(), bson.M{"domain_id": int64(20)}).Decode(&a) != nil {
					t.Fatal("fixture")
				}
				a.ID = primitive.NewObjectID()
				a.DomainID = meta.FromUint64(21)
				a.GenerationID = 999
				if _, err := db.Collection("interpret_report_artifacts").InsertOne(t.Context(), a); err != nil {
					t.Fatal(err)
				}
			case "newer_winner_original_absent":
				var r interpretmongo.InterpretationRunPO
				var a interpretmongo.InterpretReportPO
				if db.Collection("interpretation_runs").FindOne(t.Context(), bson.M{"domain_id": int64(11)}).Decode(&r) != nil || db.Collection("interpret_report_artifacts").FindOne(t.Context(), bson.M{"domain_id": int64(20)}).Decode(&a) != nil {
					t.Fatal("fixture")
				}
				r.ID = primitive.NewObjectID()
				r.DomainID = meta.FromUint64(12)
				r.Attempt = 2
				a.ID = primitive.NewObjectID()
				a.DomainID = meta.FromUint64(21)
				a.InterpretationRunID = 12
				if _, err := db.Collection("interpretation_runs").InsertOne(t.Context(), r); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Collection("interpret_report_artifacts").DeleteOne(t.Context(), bson.M{"domain_id": int64(20)}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Collection("interpret_report_artifacts").InsertOne(t.Context(), a); err != nil {
					t.Fatal(err)
				}
				update("report_generations", 1, bson.M{"latest_run_id": int64(12), "report_id": int64(21)})
			}
			var resolution *MongoOwnerResolution
			err := mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error {
				var e error
				resolution, e = ResolveMongoOwner(ctx, db, config, source, facts)
				return e
			})
			if kind == "original" || kind == "raw_mutation" {
				if err != nil || !resolution.Local().OwnerLocalTerminal || resolution.Local().ReportID != 20 || resolution.Local().OriginalRun.RunID != "11" {
					t.Fatal("original graph substituted or rejected", err)
				}
				if kind == "raw_mutation" {
					update("interpret_report_artifacts", 20, bson.M{"conclusion": "changed private body"})
					if err = mongoLocalNativeTx(t, client, func(ctx mongo.SessionContext) error { return resolution.RecheckMongo(ctx) }); !errors.Is(err, ErrMongoOwnerConflict) {
						t.Fatal("whole artifact mutation ignored", err)
					}
				}
			} else if err == nil && resolution.Local().OwnerLocalTerminal {
				t.Fatal("orphan/ambiguous/pending graph accepted")
			}
		})
	}
}
