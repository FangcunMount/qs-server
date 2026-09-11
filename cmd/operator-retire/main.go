package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorretire"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: operator-retire preflight|apply|status|verify [QS config flags]; required environment: OPERATOR_RETIRE_ID, OPERATOR_RETIRE_LAYOUT, OPERATOR_RETIRE_REPORT; apply also requires OPERATOR_RETIRE_ACTOR_ID, OPERATOR_RETIRE_FINGERPRINT, OPERATOR_RETIRE_MAINTENANCE=true")
		os.Exit(1)
	}
	action := os.Args[1]
	switch action {
	case "preflight", "apply", "status", "verify":
	default:
		fmt.Fprintln(os.Stderr, "unknown retirement action")
		os.Exit(1)
	}
	os.Args = append(os.Args[:1], os.Args[2:]...)
	opts := apiserveroptions.NewOptions()
	app.NewApp("Operator retirement with restricted evidence output", "qs-apiserver", app.WithDefaultValidArgs(), app.WithOptions(opts), app.WithRunFunc(func(_ string) error {
		log.Init(opts.Log)
		defer log.Flush()
		path := os.Getenv("OPERATOR_RETIRE_REPORT")
		if path == "" {
			return fmt.Errorf("restricted report output path required")
		}
		output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("report file must be new and writable")
		}
		defer func() { _ = output.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		conn, _, err := openDatabase(opts)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		db, ok := conn.GetClient().(*gorm.DB)
		if !ok {
			return fmt.Errorf("QS MySQL client unavailable")
		}
		module, err := container.NewIAMModuleWithRuntimeOptions(ctx, opts.IAMOptions, container.IAMModuleRuntimeOptions{})
		if err != nil {
			return fmt.Errorf("IAM connection unavailable")
		}
		defer func() { _ = module.Close() }()
		tool, err := operatorretire.New(db, module.AuthzSnapshotLoader(), iam.NewOperatorRetirementAuthzGateway(iam.NewAuthzAssignmentClient(module.Client()), module.AuthzSnapshotLoader()), os.Getenv("OPERATOR_RETIRE_LAYOUT"))
		if err != nil {
			return err
		}
		id := os.Getenv("OPERATOR_RETIRE_ID")
		actor, _ := strconv.ParseInt(os.Getenv("OPERATOR_RETIRE_ACTOR_ID"), 10, 64)
		var report *operatorretire.Report
		switch action {
		case "preflight":
			report, err = tool.Preflight(ctx, id, actor)
		case "apply":
			report, err = tool.Apply(ctx, id, os.Getenv("OPERATOR_RETIRE_FINGERPRINT"), actor, os.Getenv("OPERATOR_RETIRE_MAINTENANCE") == "true")
		case "status":
			report, err = tool.Status(ctx, id)
		case "verify":
			report, err = tool.Verify(ctx, id)
		}
		if report == nil {
			report = &operatorretire.Report{MigrationID: id, State: "invalid"}
		}
		if err != nil {
			report.LastError = err.Error()
		}
		if report != nil {
			if writeErr := json.NewEncoder(output).Encode(report); writeErr != nil {
				return writeErr
			}
			fmt.Printf("retirement state=%s candidates=%d issues=%d; detailed report saved privately\n", report.State, len(report.Candidates), len(report.Issues))
		}
		if err != nil {
			if syncErr := output.Sync(); syncErr != nil {
				return syncErr
			}
			return fmt.Errorf("retirement action stopped; inspect restricted report")
		}
		return output.Sync()
	})).Run()
}
func openDatabase(opts *apiserveroptions.Options) (*database.MySQLConnection, *sql.DB, error) {
	if opts == nil || opts.MySQLOptions == nil {
		return nil, nil, fmt.Errorf("qs MySQL options are required")
	}
	configured := opts.MySQLOptions
	connection := database.NewMySQLConnection(&database.MySQLConfig{
		Host: configured.Host, Username: configured.Username, Password: configured.Password, Database: configured.Database,
		MaxIdleConnections: 2, MaxOpenConnections: 10, MaxConnectionLifeTime: configured.MaxConnectionLifeTime,
		LogLevel: configured.LogLevel, Location: configured.Location, SessionTimeZone: configured.SessionTimeZone,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err := connection.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect QS MySQL for operator retirement")
	}
	db, ok := connection.GetClient().(*gorm.DB)
	if !ok || db == nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("resolve qs MySQL client")
	}
	sqlDB, err := db.DB()
	if err != nil {
		_ = connection.Close()
		return nil, nil, fmt.Errorf("resolve qs SQL client: %w", err)
	}
	return connection, sqlDB, nil
}
