//go:build m3_ai_paired

// Explicit isolated paired-host fixture. Never built or deployed as an application.
// Control uses one private stdin/stdout pipe; actual business traffic uses RocketMQ
// and the original mutually authenticated payload service.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	host "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	owner "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	hostopts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	payload "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/aibridge"
	options "github.com/FangcunMount/qs-server/internal/pkg/options"
	driver "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type config struct {
	Project, Source, DSN, Schema, TLSDirectory, AIPayloadEndpoint string
	Options                                                       hostopts.AIWorkflowMessagingOptions
}

type command struct {
	Mode      string     `json:"mode"`
	RequestID string     `json:"request_id"`
	CommandID string     `json:"command_id"`
	Actor     app.Actor  `json:"actor"`
	Start     app.Start  `json:"start"`
	Change    app.Change `json:"change"`
}

type routes struct{}

func (routes) GetTopicForEvent(topic string) (string, bool) {
	return topic, topic == app.CommandsTopic || topic == app.AcksTopic
}

func hash(raw []byte) string { value := sha256.Sum256(raw); return hex.EncodeToString(value[:]) }

func loopback(address string) bool {
	name, _, err := net.SplitHostPort(address)
	return err == nil && (name == "localhost" || net.ParseIP(name).IsLoopback())
}

func fail(phase string, err error) map[string]any {
	code := "unavailable"
	switch {
	case errors.Is(err, app.ErrConflict):
		code = "conflict"
	case errors.Is(err, app.ErrInvalid):
		code = "invalid"
	case errors.Is(err, app.ErrNotFound):
		code = "not_found"
	case errors.Is(err, store.ErrCommandUnresolved), errors.Is(err, store.ErrEventUnresolved):
		code = "pending"
	}
	result := map[string]any{"ok": false, "phase": phase, "code": code, "error_type": fmt.Sprintf("%T", err)}
	var db *driver.MySQLError
	if errors.As(err, &db) {
		result["mysql_code"] = db.Number
	}
	return result
}

func run() (result map[string]any) {
	phase := "private_configuration"
	result = map[string]any{"ok": false, "phase": phase}
	if len(os.Args) != 2 {
		return result
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil || len(data) > 65536 {
		return result
	}
	var cfg config
	if json.Unmarshal(data, &cfg) != nil || !strings.HasPrefix(cfg.Project, "rocketmq-m2-") ||
		!regexp.MustCompile(`^qs_m3_paired_[a-z0-9_]{8,64}$`).MatchString(cfg.Schema) ||
		!filepath.IsAbs(cfg.Source) || !filepath.IsAbs(cfg.TLSDirectory) || !loopback(cfg.AIPayloadEndpoint) {
		return result
	}
	// An isolated process must never send standard/SDK logs through the control pipe.
	sink, err := os.OpenFile(filepath.Join(cfg.TLSDirectory, "qs-private.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return result
	}
	defer sink.Close()
	log.SetOutput(sink)
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	mysql, err := driver.ParseDSN(cfg.DSN)
	if err != nil || mysql.Net != "tcp" || mysql.DBName != "" || !loopback(mysql.Addr) {
		return result
	}
	mysql.ParseTime, mysql.MultiStatements = true, true
	admin, err := sql.Open("mysql", mysql.FormatDSN())
	if err != nil {
		return fail("mysql_admin", err)
	}
	defer admin.Close()
	installCtx, installCancel := context.WithTimeout(root, 30*time.Second)
	defer installCancel()
	if _, err = admin.ExecContext(installCtx, "CREATE DATABASE `"+cfg.Schema+"`"); err != nil {
		return fail("create_owned_schema", err)
	}
	defer func() {
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if _, e := admin.ExecContext(ctx, "DROP DATABASE `"+cfg.Schema+"`"); e != nil {
			result = fail("drop_owned_schema", e)
		}
	}()
	mysql.DBName = cfg.Schema
	db, err := gorm.Open(gormmysql.Open(mysql.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fail("mysql_primary", err)
	}
	pool, err := db.DB()
	if err != nil {
		return fail("mysql_pool", err)
	}
	pool.SetMaxOpenConns(12)
	defer pool.Close()
	// Explicit fixture installation, never a constructor side effect or production DDL.
	for _, name := range []string{"000072_ai_bridge_delivery", "000083_ai_runtime_index", "000091_ai_messaging", "000092_ai_messaging_failures", "000094_ai_messaging_admission", "000095_ai_messaging_observations", "000097_ai_command_retirement", "000106_transaction_messaging", "000108_ai_transaction_command_order"} {
		raw, e := os.ReadFile(filepath.Join(cfg.Source, "internal/pkg/migration/migrations/mysql", name+".up.sql"))
		if e != nil {
			return fail("migration_source", e)
		}
		if _, e = pool.ExecContext(installCtx, string(raw)); e != nil {
			return fail("explicit_migrations", e)
		}
	}
	if cfg.Options.RocketMQ == nil || cfg.Options.SelectedProvider() != "rocketmq" {
		return result
	}
	binding, ok := cfg.Options.RocketMQ.Transactions["qs.mysql"]
	if !ok {
		return result
	}
	if _, err = pool.ExecContext(installCtx, "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte("qs.mysql"), []byte(binding.StoreID), []byte(binding.WriterEpoch)); err != nil {
		return fail("owner_binding", err)
	}
	if _, err = pool.ExecContext(installCtx, "UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1"); err != nil {
		return fail("fixture_admission", err)
	}
	keys, err := host.LoadMessagingKeys(cfg.Options)
	if err != nil {
		return fail("local_workload_keys", err)
	}
	var seals atomic.Int64
	seal := func(kind pb.MessagingKind, id, aggregate, org, at string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
		seals.Add(1)
		if at == "" {
			at = time.Now().UTC().Format(time.RFC3339Nano)
		}
		return app.ProtectMessaging(kind, id, aggregate, "", org, at, body, keys.Signing, keys.Recipient)
	}
	shared, err := owner.New(db, &options.MessagingOptions{Enabled: true, Provider: "rocketmq", RocketMQ: cfg.Options.RocketMQ}, routes{}, []string{app.CommandsTopic, app.AcksTopic})
	if err != nil {
		return fail("shared_owner", err)
	}
	commands, err := store.NewTransactionCommands(db, shared, seal)
	if err != nil {
		return fail("original_commands", err)
	}
	legacy := store.NewMessagingStore()
	events, err := store.NewTransactionEvents(db, shared, legacy, seal, binding)
	if err != nil {
		return fail("original_events", err)
	}
	ca, err := os.ReadFile(filepath.Join(cfg.TLSDirectory, "ca.pem"))
	if err != nil {
		return fail("payload_ca", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return result
	}
	certificate, err := tls.LoadX509KeyPair(filepath.Join(cfg.TLSDirectory, "qs.pem"), filepath.Join(cfg.TLSDirectory, "qs.key"))
	if err != nil {
		return fail("payload_certificate", err)
	}
	conn, err := grpc.NewClient(cfg.AIPayloadEndpoint, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "localhost", Certificates: []tls.Certificate{certificate}})))
	if err != nil {
		return fail("payload_client", err)
	}
	defer conn.Close()
	reader := host.NewMessagingPayloadClient(conn)
	reader.Observe = func(ctx context.Context, kind string) error { return store.RecordPayloadObservation(ctx, pool, kind) }
	receiver := &store.MessagingEventReceiver{DB: pool, Store: legacy, Bodies: reader, Seal: seal, Keys: keys.Ring, Transactions: events}
	runtime, err := host.NewMessagingRuntime(cfg.Options, pool, legacy, receiver)
	if err != nil {
		return fail("event_runtime", err)
	}
	runtime.CommandTransactions = commands
	var server *grpc.Server
	var listener net.Listener
	var serveDone chan error
	// Keep the payload server available while the actual consumer and owner drain.
	defer func() {
		if server == nil {
			return
		}
		server.Stop()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(5 * time.Second):
			result = fail("payload_close", context.DeadlineExceeded)
		}
	}()
	if err = shared.Runtime().Start(root); err != nil {
		return fail("owner_start", err)
	}
	defer func() {
		ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		if e := shared.Runtime().Close(ctx); e != nil {
			result = fail("owner_close", e)
		}
	}()
	if err = shared.Runtime().SetAdmission(true); err != nil {
		return fail("owner_admission", err)
	}
	if err = runtime.Start(root); err != nil {
		return fail("consumer_start", err)
	}
	defer func() {
		ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		if e := runtime.Stop(ctx); e != nil {
			result = fail("consumer_close", e)
		}
	}()
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail("payload_listener", err)
	}
	server = grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert})))
	(&payload.MessagePayloads{Reader: &store.MessagingReader{DB: pool, Store: legacy, Archive: commands.Archive()}}).RegisterService(server)
	serveDone = make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	commandStore := &store.MessagingCommandStore{Store: &store.Store{DB: pool}, Messaging: legacy, Seal: seal, Transactions: commands}
	service := &app.Service{Store: commandStore}
	output := json.NewEncoder(os.Stdout)
	if err = output.Encode(map[string]any{"ready": true, "schema": cfg.Schema, "payload_endpoint": listener.Addr().String()}); err != nil {
		return fail("control_ready", err)
	}
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 3<<20)
	for input.Scan() {
		var c command
		if json.Unmarshal(input.Bytes(), &c) != nil {
			if err = output.Encode(map[string]any{"ok": false, "code": "invalid_control"}); err != nil {
				return fail("control_write", err)
			}
			continue
		}
		if c.Mode == "close" {
			return map[string]any{"ok": true, "closed": true}
		}
		ctx, end := context.WithTimeout(root, 20*time.Second)
		response := map[string]any{"ok": true}
		switch c.Mode {
		case "start":
			err = service.Start(ctx, c.Start)
		case "change":
			err = service.Change(ctx, c.RequestID, c.Change)
		case "facts":
			var op app.MessagingOperation
			op, err = commandStore.ReadRequestOperation(ctx, app.OperationScope{OrganizationID: c.Actor.OrgID, SubjectID: c.Actor.SubjectID}, c.RequestID, c.CommandID)
			if err == nil {
				response["operation"] = op
			}
			if err == nil {
				var projection *app.Event
				projection, err = commandStore.Projection(ctx, c.RequestID)
				if err == nil && projection != nil {
					response["projection"] = map[string]any{"session_id": projection.SessionID, "version": projection.Version, "status": projection.Status, "question_id": projection.QuestionID, "question_sha256": hash([]byte(projection.Question)), "question_length": len(projection.Question)}
				}
			}
			if err == nil {
				var archive app.FrozenMessagingArchive
				var found bool
				archive, found, err = commands.Archive().ReadCommitted(ctx, c.CommandID)
				if err == nil && found {
					response["command"] = map[string]any{"wire_sha256": hash(archive.Wire), "body_sha256": hash(archive.Body), "body_length": len(archive.Body), "reference": archive.Envelope.GetPayloadReference() != nil}
				}
			}
			if err == nil {
				var rows *sql.Rows
				rows, err = pool.QueryContext(ctx, "SELECT message_id FROM ai_messaging_archive ORDER BY message_id")
				ids := []string{}
				if err == nil {
					for rows.Next() {
						var id string
						if err = rows.Scan(&id); err != nil {
							break
						}
						ids = append(ids, id)
					}
					if err == nil {
						err = rows.Err()
					}
					_ = rows.Close()
				}
				values := []map[string]any{}
				for _, id := range ids {
					if err != nil {
						break
					}
					var a app.FrozenMessagingArchive
					var found bool
					a, found, err = commands.Archive().ReadCommitted(ctx, id)
					if err == nil && !found {
						err = app.ErrMessagingContract
					}
					if err == nil {
						values = append(values, map[string]any{"id": id, "kind": a.Envelope.Kind, "wire_sha256": hash(a.Wire), "body_sha256": hash(a.Body)})
					}
				}
				response["archives"] = values
			}
			if err == nil {
				counts := map[string]int{}
				for _, table := range []string{"ai_messaging_operations", "ai_messaging_inbox", "ai_messaging_outbox", "ai_messaging_archive", "rm_tx_execution", "rm_tx_witness"} {
					var n int
					if err = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
						break
					}
					counts[table] = n
				}
				response["counts"] = counts
				response["seals"] = seals.Load()
			}
		default:
			err = app.ErrInvalid
		}
		end()
		if err != nil {
			response = fail(c.Mode, err)
		}
		if err = output.Encode(response); err != nil {
			return fail("control_write", err)
		}
	}
	if err = input.Err(); err != nil {
		return fail("control_read", err)
	}
	return map[string]any{"ok": true, "closed": true}
}

func main() {
	result := run()
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(1)
	}
	if result["ok"] != true {
		os.Exit(1)
	}
}
