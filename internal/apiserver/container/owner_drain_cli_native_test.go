//go:build integration && m3_host_runtime

package container

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/message"
	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"gorm.io/gorm"
)

// Actual standalone maintenance executable, not an in-process lifecycle stub.
// All databases, messages and the Broker process belong to the native fixture.
func TestM5OwnerDrainCLI(t *testing.T) {
	data := ownedDefaultOwnerData(t)
	ownedHostBroker(t, "hold-checks")
	var opts generic.MessagingOptions
	raw, err := os.ReadFile(os.Getenv("QS_M3_HOST_OPTIONS"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &opts))
	for _, file := range []string{"000106_transaction_messaging.up.sql", "000107_business_retry_schedule.up.sql", "000110_native_transaction_limit_notices.up.sql"} {
		b, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, e)
		_, e = data.pool.ExecContext(t.Context(), string(b))
		require.NoError(t, e)
	}
	for _, file := range []string{"000041_transaction_messaging.up.json", "000042_business_retry_schedule.up.json", "000043_native_transaction_limit_notices.up.json"} {
		b, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mongodb", file))
		require.NoError(t, e)
		var commands []json.RawMessage
		require.NoError(t, json.Unmarshal(b, &commands))
		for _, command := range commands {
			var doc bson.D
			require.NoError(t, bson.UnmarshalExtJSON(command, false, &doc))
			require.NoError(t, data.documents.RunCommand(t.Context(), doc).Err())
		}
	}
	_, err = data.pool.ExecContext(t.Context(), "CREATE TABLE schema_migrations(version BIGINT PRIMARY KEY,dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(110,FALSE); CREATE TABLE m5_drain_facts(id VARCHAR(128) PRIMARY KEY,wire_hash BINARY(32) NOT NULL)")
	require.NoError(t, err)
	_, err = data.documents.Collection("schema_migrations").InsertOne(t.Context(), bson.M{"version": 44, "dirty": false})
	require.NoError(t, err)
	runtimes := []*messagingruntime.TransactionRuntime{}
	var calls [2]atomic.Int32
	var wireHashes [2][32]byte
	committed := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var work sync.WaitGroup
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		finished := make(chan struct{})
		go func() { work.Wait(); close(finished) }()
		select {
		case <-finished:
			for _, r := range runtimes {
				q, c := context.WithTimeout(context.Background(), 30*time.Second)
				require.NoError(t, r.Close(q))
				c()
			}
		case <-time.After(30 * time.Second):
			t.Error("owned drain callbacks did not join")
		}
	})
	results := make(chan transaction.Result, 2)
	for i, owner := range []string{"qs.mysql", "qs.mongo"} {
		b := opts.RocketMQ.Transactions[owner]
		binding := transaction.Binding{Owner: owner, StoreID: b.StoreID, WriterEpoch: b.WriterEpoch}
		var store transaction.PreparedStore
		var native messagingruntime.NativeLimitReader
		if i == 0 {
			_, err = data.pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(owner), []byte(b.StoreID), []byte(b.WriterEpoch))
			require.NoError(t, err)
			store, err = txmysql.New(data.pool, binding)
			require.NoError(t, err)
			native, err = messagingruntime.NewSQLNativeLimits(data.pool, binding, "m3-1")
		} else {
			_, err = data.documents.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": owner, "store_id": b.StoreID, "writer_epoch": b.WriterEpoch, "schema_version": 1})
			require.NoError(t, err)
			store, err = txmongo.New(data.documents, binding)
			require.NoError(t, err)
			native, err = messagingruntime.NewMongoNativeLimits(data.documents, binding, "m3-1")
		}
		require.NoError(t, err)
		r, e := messagingruntime.NewTransactionalSender(&opts, owner, store, []string{"qs.evaluation.lifecycle"}, native)
		require.NoError(t, e)
		require.NoError(t, r.Start(t.Context()))
		runtimes = append(runtimes, r)
		id := fmt.Sprintf("m5-drain-cli-%d-%d", i, time.Now().UnixNano())
		payload := []byte(id)
		wire, e := legacy.Encode(legacy.Envelope{UUID: id, Payload: payload, Metadata: map[string]string{"fixture": "m5-owner-drain"}}, legacy.Revision2)
		require.NoError(t, e)
		wireHashes[i] = sha256.Sum256(wire)
		msg, e := message.New(message.Input{Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle", EventType: "m5.drain.fixture", SchemaVersion: "1", Scope: "fixture", ContentType: "application/octet-stream", OccurredAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: payload})
		require.NoError(t, e)
		physical, e := transport.PhysicalTopicV2("qs.evaluation.lifecycle", transport.TransactionMessageType, owner, "m3-1")
		require.NoError(t, e)
		plan, e := transaction.NewPlan(transaction.PlanInput{Owner: owner, Scope: "fixture", OperationID: id, IdempotencyKey: id, InputHash: sha256.Sum256(payload), Branches: []transaction.BranchInput{{ID: "event", PhysicalTopic: physical, RouteRevision: "m3-1", Message: msg, Wire: wire}}})
		require.NoError(t, e)
		reservation, e := r.Reserve(t.Context(), transaction.Request{Owner: owner, Scope: "fixture", Key: id, InputHash: sha256.Sum256(payload)}, plan)
		require.NoError(t, e)
		require.True(t, reservation.Acquired)
		work.Add(1)
		go func(i int, r *messagingruntime.TransactionRuntime, store transaction.PreparedStore, execution transaction.Execution, wire []byte, id string) {
			defer work.Done()
			result, _ := r.Execute(t.Context(), execution, func(ctx context.Context, x transaction.Execution) (transaction.LocalOutcome, error) {
				calls[i].Add(1)
				hash := sha256.Sum256(wire)
				var e error
				if i == 0 {
					e = data.sql.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
						guard, e := store.(*txmysql.Store).BindGORM(ctx, tx, x)
						if e != nil {
							return e
						}
						if e = tx.Exec("INSERT INTO m5_drain_facts VALUES(?,?)", id, hash[:]).Error; e != nil {
							return e
						}
						return guard.CommitWitness(ctx)
					})
				} else {
					session, se := data.documents.Client().StartSession()
					if se != nil {
						return transaction.LocalUnknown, se
					}
					defer session.EndSession(context.Background())
					_, e = session.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
						guard, e := store.(*txmongo.Store).Bind(sc, x)
						if e != nil {
							return nil, e
						}
						if _, e = data.documents.Collection("m5_drain_facts").InsertOne(sc, bson.M{"_id": id, "wire_hash": hash[:]}); e != nil {
							return nil, e
						}
						return nil, guard.CommitWitness(sc)
					}, mongooptions.Transaction().SetWriteConcern(writeconcern.Majority()))
				}
				if e != nil {
					return transaction.LocalUnknown, e
				}
				committed <- struct{}{}
				<-release
				return transaction.LocalCommitted, nil
			})
			results <- result
		}(i, r, store, reservation.Execution, wire, id)
	}
	for range 2 {
		select {
		case <-committed:
		case <-time.After(30 * time.Second):
			t.Fatal("owned root did not commit")
		}
	}
	ownedHostBroker(t, "kill")
	releaseOnce.Do(func() { close(release) })
	for range 2 {
		select {
		case r := <-results:
			require.Equal(t, transaction.LocalCommitted, r.Local)
			require.Equal(t, transaction.FinalUnknown, r.Branches[0].Final)
		case <-time.After(30 * time.Second):
			t.Fatal("unknown final did not finish")
		}
	}
	ownedHostBroker(t, "start")
	waitOwnedHostTLS(t, opts.RocketMQ)
	ownedHostBroker(t, "hold-checks")
	var observer generic.MessagingOptions
	raw, err = os.ReadFile(os.Getenv("QS_M3_HOST_OBSERVER_OPTIONS"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &observer))
	_, err = waitOwnedProxyRoutes(t.Context(), opts.RocketMQ, observer.RocketMQ)
	require.NoError(t, err)
	dir := t.TempDir()
	binary := filepath.Join(dir, "qs-migrate-offline")
	root := filepath.Clean("../../..")
	shaBytes, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	source := strings.TrimSpace(string(shaBytes))
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags", "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+source, "-o", binary, "./scripts/maintenance/qs-migrate-offline")
	build.Dir = root
	require.NoError(t, build.Run())
	config, err := json.Marshal(map[string]any{"messaging": opts})
	require.NoError(t, err)
	configFile := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(configFile, config, 0600))
	hash := sha256.Sum256(config)
	pair := migration.PairConfig{MySQLDatabase: data.documents.Name(), MongoDatabase: data.documents.Name()}
	sqlID, mongoID, err := migration.InspectPairIdentity(t.Context(), data.pool, data.documents.Client(), pair)
	require.NoError(t, err)
	dsn, err := driver.ParseDSN(os.Getenv("QS_M3_HOST_DSN"))
	require.NoError(t, err)
	dsn.DBName = data.documents.Name()
	op := fmt.Sprintf("%d-1", time.Now().UnixNano())
	command := exec.Command(binary, "--mysql-database="+pair.MySQLDatabase, "--mongo-database="+pair.MongoDatabase, "--timeout=3m", "--owner-drain", "--owner-drain-config="+configFile, "--owner-drain-config-sha256="+hex.EncodeToString(hash[:]), "--owner-drain-operation="+op, "--owner-drain-mysql-identity="+sqlID, "--owner-drain-mongo-identity="+mongoID)
	command.Env = append(os.Environ(), "TMPDIR="+dir, "QS_OFFLINE_MYSQL_DSN="+dsn.FormatDSN(), "QS_OFFLINE_MONGO_URI="+os.Getenv("QS_M3_HOST_MONGO_URI"), "MYSQL_TLS_MODE=DISABLED", "MYSQL_TLS_CA_FILE=")
	log, err := os.OpenFile(filepath.Join(dir, "private-client.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err)
	defer log.Close()
	command.Stdout, command.Stderr = log, log
	require.NoError(t, command.Start())
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() { done <- command.Wait(); close(exited) }()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = command.Process.Kill()
			<-exited
		}
	})
	type sample struct {
		Phase   string `json:"phase"`
		Expired bool   `json:"expired"`
		Owners  []struct {
			SDK      rocketmq.TransactionStats       `json:"sdk"`
			Protocol transaction.ProtocolObservation `json:"protocol"`
		} `json:"owners"`
	}
	statusFile := filepath.Join(dir, "qs-owner-drain-"+op, "status.json")
	read := func() (s sample) {
		raw, e := os.ReadFile(statusFile)
		if e == nil {
			_ = json.Unmarshal(raw, &s)
		}
		return s
	}
	require.Eventually(t, func() bool {
		s := read()
		return len(s.Owners) == 2 && s.Owners[0].SDK.Ready && s.Owners[1].SDK.Ready && s.Owners[0].Protocol.ConfirmationGapBranches > 0 && s.Owners[1].Protocol.ConfirmationGapBranches > 0
	}, 40*time.Second, 100*time.Millisecond)
	// Production registers maintenance before stopping the old owners. Prove
	// both original SDKs are still live, then close them while the independent
	// CLI retains the last actual Checkers and unresolved primary responsibility.
	for _, r := range runtimes {
		require.True(t, r.SDKStats().Ready)
		require.NoError(t, r.Close(t.Context()))
		require.True(t, r.SDKStats().Stopped)
	}
	require.Eventually(t, func() bool { s := read(); return len(s.Owners) == 2 && s.Owners[0].SDK.Ready && s.Owners[1].SDK.Ready }, 10*time.Second, 100*time.Millisecond)
	stop := []byte(`{"operation_id":"` + op + `","writers_stopped":true,"replacement_owner_ready":false,"receipt_sha256":"` + strings.Repeat("a", 64) + `"}`)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(statusFile), "stop.json"), stop, 0600))
	require.NoError(t, command.Process.Signal(syscall.SIGTERM))
	require.Eventually(t, func() bool { s := read(); return s.Phase == "stop_refused_unresolved" && s.Expired }, 10*time.Second, 100*time.Millisecond)
	select {
	case <-done:
		t.Fatal("unresolved last Checkers exited on signal")
	default:
	}
	ownedHostBroker(t, "release-checks")
	select {
	case e := <-done:
		require.NoError(t, e)
	case <-time.After(90 * time.Second):
		t.Fatal("real CLI Checker recovery did not finish")
	}
	last := read()
	require.Equal(t, "stopped", last.Phase)
	require.Len(t, last.Owners, 2)
	for _, o := range last.Owners {
		require.True(t, o.SDK.Stopped)
		require.Zero(t, o.SDK.HalfPreparedResults)
		require.Positive(t, o.SDK.CheckCommit)
		require.Positive(t, o.SDK.RecoveryAuditRecorded)
		require.Zero(t, o.Protocol.ConfirmationGapBranches)
	}
	var facts, head int
	var dirty bool
	require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM m5_drain_facts").Scan(&facts))
	require.Equal(t, 1, facts)
	n, err := data.documents.Collection("m5_drain_facts").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT version,dirty FROM schema_migrations").Scan(&head, &dirty))
	require.Equal(t, 110, head)
	require.False(t, dirty)
	var originalWire []byte
	require.NoError(t, data.pool.QueryRowContext(t.Context(), "SELECT wire_hash FROM m5_drain_facts").Scan(&originalWire))
	require.Equal(t, wireHashes[0][:], originalWire)
	var originalMongo struct {
		Hash []byte `bson:"wire_hash"`
	}
	require.NoError(t, data.documents.Collection("m5_drain_facts").FindOne(t.Context(), bson.D{}).Decode(&originalMongo))
	require.Equal(t, wireHashes[1][:], originalMongo.Hash)
	var mongoHead struct {
		Version int  `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	require.NoError(t, data.documents.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&mongoHead))
	require.Equal(t, 44, mongoHead.Version)
	require.False(t, mongoHead.Dirty)
	for i := range calls {
		require.EqualValues(t, 1, calls[i].Load())
	}
	receipt, err := json.Marshal(map[string]any{"source_sha": source, "passed": 9, "failed": 0, "skipped": 0, "owner_count": 2, "actual_CLI": true, "config_and_pair_bound": true, "actual_Broker_unknown_final_and_recovery": true, "unresolved_signal_stop_refused": true, "no_new_halves": true, "original_business_facts_single": true, "original110_44_retained": true, "actual_SDK_and_process_exit": true, "live_original_overlap_before_old_close": true, "production_or_system_acceptance": false})
	require.NoError(t, err)
	fmt.Println("M5_QS_OWNER_DRAIN_CLI_RECEIPT " + string(receipt))
}
