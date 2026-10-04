// ai-mq-process is an isolated acceptance probe, never a production entrypoint.
// It uses the host runtime and original command store without a second relay.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	client "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "isolated MQ process probe failed")
		os.Exit(1)
	}
}

func run() (resultErr error) {
	mode := flag.String("mode", "runtime", "runtime, stage-cancel or stage-history")
	input := flag.String("input", "", "disposable historical request fixture")
	config := flag.String("config", "", "disposable local configuration file")
	id := flag.String("id", "", "original command UUID")
	agg := flag.String("aggregate", "", "original run UUID")
	flag.Parse()
	var cfg struct {
		Options                opts.AIWorkflowMessagingOptions
		Address, CA, Cert, Key string
	}
	raw, err := os.ReadFile(*config)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	keys, err := client.LoadMessagingKeys(cfg.Options)
	if err != nil {
		return err
	}
	db, err := sql.Open("mysql", os.Getenv("QS_AI_MQ_PROBE_DSN"))
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	db.SetMaxOpenConns(5)
	if *mode == "stage-history" {
		// This setup models a previously accepted request. It does not claim
		// production authorization or model execution acceptance.
		var fixture struct {
			Request app.Start
			Receipt app.Receipt
		}
		raw, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &fixture); err != nil {
			return err
		}
		// Historical state is seeded only by this isolated acceptance binary;
		// the production Store has no command writer or receipt settlement.
		if fixture.Receipt.SessionID == "" || fixture.Receipt.Version < 1 {
			return app.ErrInvalid
		}
		raw, err = json.Marshal(fixture.Request)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		r := fixture.Request
		if _, err = tx.Exec("INSERT INTO ai_bridge_requests(request_id,request_hash,payload,organization_id,subject_id,testee_id,session_id,created_at,updated_at) VALUES(?,?,CONVERT(CAST(? AS BINARY) USING utf8mb4),?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", r.RequestID, hash, raw, r.Actor.OrgID, r.Actor.SubjectID, r.TesteeID, fixture.Receipt.SessionID); err != nil {
			return err
		}
		for _, assessment := range r.AssessmentIDs {
			if _, err = tx.Exec("INSERT INTO ai_bridge_request_assessments(request_id,assessment_id) VALUES(?,?)", r.RequestID, assessment); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,available_at,delivered) VALUES(?,?,'start',CONVERT(CAST(? AS BINARY) USING utf8mb4),?,UTC_TIMESTAMP(6),TRUE)", r.RequestID, r.RequestID, raw, hash); err != nil {
			return err
		}
		return tx.Commit()
	}
	s := store.NewMessagingStore()
	seal := func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		if at == "" {
			at = time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano)
		}
		return app.ProtectMessaging(k, id, agg, "", org, at, b, keys.Signing, keys.Recipient)
	}
	if *mode == "stage-cancel" {
		no := false
		commands := &store.MessagingCommandStore{Store: &store.Store{DB: db}, Messaging: s, Seal: seal}
		return commands.SubmitEvaluationCancel(context.Background(), app.EvaluationScope{OrganizationID: 1, OperatorUserID: 42, RunID: *agg}, *id, app.EvaluationCancel{CommandID: *id, ExpectedVersion: 1, Reason: "isolated MQ fault acceptance", Confirm: true, Discard: &no})
	}
	if *mode != "runtime" {
		return errors.New("unsupported probe mode")
	}
	ca, err := os.ReadFile(cfg.CA)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid test CA")
	}
	pair, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient(cfg.Address, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{pair}, ServerName: "localhost"})), grpc.WithDisableRetry())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, conn.Close()) }()
	r, err := client.NewMessagingRuntime(cfg.Options, db, s, &store.MessagingEventReceiver{DB: db, Store: s, Keys: keys.Ring, Bodies: client.NewMessagingPayloadClient(conn), Seal: seal})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if err = r.Start(ctx); err != nil {
		return err
	}
	fmt.Println("READY")
	<-ctx.Done()
	stop, end := context.WithTimeout(context.Background(), 20*time.Second)
	defer end()
	return r.Stop(stop)
}
