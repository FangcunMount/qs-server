package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operatorretirement"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorrecovery"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: operator-recover preflight|apply|status [QS config flags]; OPERATOR_RECOVERY_INPUT and OPERATOR_RECOVERY_REPORT required; apply also requires OPERATOR_RECOVERY_FINGERPRINT and OPERATOR_RECOVERY_MAINTENANCE=true")
		os.Exit(1)
	}
	action := os.Args[1]
	switch action {
	case "preflight", "apply", "status":
	default:
		fmt.Fprintln(os.Stderr, "unknown recovery action")
		os.Exit(1)
	}
	os.Args = append(os.Args[:1], os.Args[2:]...)
	opts := apiserveroptions.NewOptions()
	app.NewApp("Audited operator identity recovery", "qs-apiserver", app.WithDefaultValidArgs(), app.WithOptions(opts), app.WithRunFunc(func(_ string) error {
		log.Init(opts.Log)
		defer log.Flush()
		cmd, err := readCommand(os.Getenv("OPERATOR_RECOVERY_INPUT"))
		if err != nil {
			return err
		}
		cmd.WritesStopped = os.Getenv("OPERATOR_RECOVERY_MAINTENANCE") == "true"
		output, err := os.OpenFile(os.Getenv("OPERATOR_RECOVERY_REPORT"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("new restricted report path required")
		}
		defer func() { _ = output.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
		tool := operatorrecovery.New(db, module.AuthzSnapshotLoader(), iam.NewOperatorRetirementAuthzGateway(iam.NewAuthzAssignmentClient(module.Client()), module.AuthzSnapshotLoader()))
		var report *operatorrecovery.Report
		switch action {
		case "preflight":
			report, err = tool.Preflight(ctx, cmd)
		case "status":
			report, err = tool.Status(ctx, cmd)
		case "apply":
			report, err = tool.Apply(ctx, cmd, os.Getenv("OPERATOR_RECOVERY_FINGERPRINT"))
		}
		if report == nil {
			report = &operatorrecovery.Report{State: "invalid", Command: cmd}
		}
		if err != nil {
			report.LastError = err.Error()
		}
		if e := json.NewEncoder(output).Encode(report); e != nil {
			return e
		}
		if e := output.Sync(); e != nil {
			return e
		}
		if e := output.Close(); e != nil {
			return e
		}
		fmt.Printf("operator recovery state=%s; details saved privately\n", report.State)
		if err != nil {
			return fmt.Errorf("recovery stopped; inspect restricted report")
		}
		return nil
	})).Run()
}

func readCommand(path string) (operatorapp.RecoveryCommand, error) {
	var cmd operatorapp.RecoveryCommand
	file, err := os.Open(path)
	if err != nil {
		return cmd, fmt.Errorf("recovery input file required")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return cmd, fmt.Errorf("recovery input must be a regular file smaller than 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("invalid recovery input")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return cmd, fmt.Errorf("recovery input must contain one JSON object")
	}
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || cmd.OperatorID == 0 || cmd.ExpectedVersion == 0 || cmd.ExpectedPolicyVersion <= 0 || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || strings.TrimSpace(cmd.Reason) != cmd.Reason || cmd.Reason == "" || len([]rune(cmd.Reason)) > 500 {
		return cmd, fmt.Errorf("exact recovery identity, versions, request and reason required")
	}
	return cmd, nil
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
		return nil, nil, fmt.Errorf("connect QS MySQL for operator recovery")
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
