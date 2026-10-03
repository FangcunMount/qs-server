// Explicit maintenance-only migration. The audit executable remains read-only.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	client "github.com/FangcunMount/qs-server/internal/apiserver/infra/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	maintenance "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/aimessaginghandoff"
	options "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/go-sql-driver/mysql"
)

var sourceSHA = "development"

func main() {
	action := flag.String("action", "dry-run", "dry-run or apply; never publishes messages")
	input := flag.String("input", "", "explicit IDs JSON for dry-run or reviewed manifest JSON for apply")
	digest := flag.String("reviewed-sha256", "", "exact canonical manifest digest required for apply")
	keys := flag.String("key-options", "", "local JSON AIWorkflowMessagingOptions required only for apply")
	stopped := flag.Bool("confirm-relays-stopped", false, "operator has verified all old/new delivery claimers stopped")
	flag.Parse()
	if err := run(*action, *input, *digest, *keys, *stopped); err != nil {
		// Never disclose DSN, source body, key content or raw storage errors.
		fmt.Fprintln(os.Stderr, "MQ handoff stopped; inspect partial report and original identities before resuming")
		os.Exit(1)
	}
}

func decode(path string, target any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1<<20 {
		return maintenance.ErrManifest
	}
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err = d.Decode(&extra); !errors.Is(err, io.EOF) {
		return maintenance.ErrManifest
	}
	return nil
}

func run(action, input, digest, keyFile string, stopped bool) error {
	if action != "dry-run" && action != "apply" {
		return maintenance.ErrManifest
	}
	if input == "" || (action == "dry-run" && (digest != "" || keyFile != "" || stopped)) {
		return maintenance.ErrManifest
	}
	cfg, err := mysql.ParseDSN(os.Getenv("QS_AI_MESSAGING_HANDOFF_DSN"))
	if err != nil || cfg.DBName == "" {
		return maintenance.ErrManifest
	}
	cfg.ParseTime, cfg.Loc = true, time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tool := &maintenance.Tool{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if action == "dry-run" {
		var ids []string
		if err = decode(input, &ids); err != nil {
			return err
		}
		m, err := tool.DryRun(ctx, ids)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			SourceSHA string               `json:"source_sha"`
			ReadOnly  bool                 `json:"read_only"`
			Manifest  maintenance.Manifest `json:"manifest"`
			SHA256    string               `json:"manifest_sha256"`
		}{sourceSHA, true, m, m.SHA256()})
	}
	var m maintenance.Manifest
	if err = decode(input, &m); err != nil {
		return err
	}
	if digest != m.SHA256() || !stopped {
		return maintenance.ErrManifest
	}
	var opts options.AIWorkflowMessagingOptions
	if err = decode(keyFile, &opts); err != nil {
		return err
	}
	keys, err := client.LoadMessagingKeys(opts)
	if err != nil {
		return err
	}
	tool.Handoff = &store.MessagingLegacyHandoff{Store: store.NewMessagingStore(), Seal: func(kind pb.MessagingKind, id, aggregate, org, at string, body *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return app.ProtectMessaging(kind, id, aggregate, "", org, at, body, keys.Signing, keys.Recipient)
	}}
	result, applyErr := tool.Apply(ctx, m, digest, stopped)
	if err = json.NewEncoder(os.Stdout).Encode(struct {
		SourceSHA string             `json:"source_sha"`
		Complete  bool               `json:"complete"`
		Result    maintenance.Result `json:"result"`
	}{sourceSHA, applyErr == nil, result}); err != nil {
		return err
	}
	return applyErr
}
