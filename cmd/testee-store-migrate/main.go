// testee-store-migrate maintains audited initial store ownership.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	migration "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/testeestore"
	mysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 || (os.Args[1] != "preflight" && os.Args[1] != "status" && os.Args[1] != "verify" && os.Args[1] != "apply" && os.Args[1] != "rollback") {
		return fmt.Errorf("usage: testee-store-migrate preflight --output <private-new-file> | status/verify --migration-id <id> | apply/rollback --migration-id <id> --fingerprint <original> --org-id <company> --actor-id <operator-user> --writes-paused --output <private-new-file>; DSN via TESTEE_STORE_MYSQL_DSN")
	}
	action := os.Args[1]
	write := action == "apply" || action == "rollback"
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	migrationID := flags.String("migration-id", "", "independent ownership migration identifier")
	output := flags.String("output", "", "new private JSON report path")
	limit := flags.Int("max-rows", 100000, "maximum rows per source table; exceeding it stops preflight")
	fingerprint := flags.String("fingerprint", "", "original preflight fingerprint")
	org := flags.Int64("org-id", 0, "company ID for the audited migration")
	actor := flags.Int64("actor-id", 0, "active headquarters Operator IAM UserID")
	paused := flags.Bool("writes-paused", false, "confirm ownership and relationship writes are paused")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum operation duration")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if ((action == "preflight" || write) && *output == "") || (action != "preflight" && *migrationID == "") || flags.NArg() != 0 {
		return fmt.Errorf("output file required; unexpected positional arguments rejected")
	}
	if *timeout <= 0 || *limit < 1 || *limit > 1000000 {
		return fmt.Errorf("invalid operation timeout or row limit")
	}
	var iamOptions *iam.IAMOptions
	if write {
		if *org <= 0 || *actor <= 0 || !*paused || *fingerprint == "" {
			return fmt.Errorf("write requires company, actor, original fingerprint and paused writes")
		}
		var err error
		iamOptions, err = maintenanceIAMOptions(os.Getenv)
		if err != nil {
			return err
		}
	}
	dsn := os.Getenv("TESTEE_STORE_MYSQL_DSN")
	if dsn == "" {
		return fmt.Errorf("TESTEE_STORE_MYSQL_DSN required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("cannot connect to ownership database")
	}
	conn, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if write {
		// Reserve evidence before any mutation. Existing reports are never overwritten.
		f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("cannot create new private execution report")
		}
		defer func() { _ = f.Close() }()
		client, err := iam.NewClientWithRuntimeOptions(ctx, iamOptions, iam.ClientRuntimeOptions{})
		if err != nil {
			return fmt.Errorf("maintenance IAM connection unavailable")
		}
		defer func() { _ = client.Close() }()
		authorized, err := authorizeMaintenance(ctx, iam.NewAuthzSnapshotLoader(client, iam.AuthzSnapshotLoaderOptions{AppName: "qs"}), *actor)
		if err != nil {
			return err
		}
		command := migration.ApplyCommand{MigrationID: *migrationID, Fingerprint: *fingerprint, OrgID: *org, ActorID: *actor, MaxRows: *limit, WritesPaused: *paused}
		var manifest *migration.Manifest
		if action == "apply" {
			manifest, err = migration.Apply(authorized, db, command)
		} else {
			manifest, err = migration.Rollback(authorized, db, command)
		}
		detail := struct {
			Manifest *migration.Manifest `json:"manifest"`
			Error    string              `json:"error,omitempty"`
		}{Manifest: manifest}
		if err != nil {
			detail.Error = err.Error()
		}
		if e := json.NewEncoder(f).Encode(detail); e != nil {
			return fmt.Errorf("execution report failed; inspect database status before retrying")
		}
		if e := f.Sync(); e != nil {
			return fmt.Errorf("execution report sync failed; inspect database status before retrying")
		}
		if err != nil {
			return fmt.Errorf("migration stopped; inspect private execution report")
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			State       string `json:"state"`
			MigrationID string `json:"migration_id"`
		}{manifest.State, manifest.MigrationID})
	}
	if action != "preflight" {
		var result migration.Status
		var statusErr error
		if action == "verify" {
			result, statusErr = migration.Verify(ctx, db, *migrationID, *limit)
		} else {
			result, statusErr = migration.ReadStatus(ctx, db, *migrationID, *limit)
		}
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return err
		}
		return statusErr
	}
	report, err := migration.Preflight(ctx, db, *limit)
	if err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("cannot create new private report: %w", err)
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(report)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(*output)
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Fingerprint string         `json:"fingerprint"`
		SchemaReady bool           `json:"schema_ready"`
		Executable  bool           `json:"executable"`
		Counts      map[string]int `json:"counts"`
	}{report.Fingerprint, report.SchemaReady, report.Executable, report.Counts})
}
