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
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorprepare"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: operator-prepare preflight|apply|status [QS config flags]; OPERATOR_PREPARE_INPUT and OPERATOR_PREPARE_REPORT required; apply also requires OPERATOR_PREPARE_FINGERPRINT")
		os.Exit(1)
	}
	action := os.Args[1]
	switch action {
	case "preflight", "apply", "status":
	default:
		fmt.Fprintln(os.Stderr, "unknown preparation action")
		os.Exit(1)
	}
	os.Args = append(os.Args[:1], os.Args[2:]...)
	opts := apiserveroptions.NewOptions()
	app.NewApp("Inactive operator identity preparation", "qs-apiserver", app.WithDefaultValidArgs(), app.WithOptions(opts), app.WithRunFunc(func(_ string) error {
		log.Init(opts.Log)
		defer log.Flush()
		cmd, err := readCommand(os.Getenv("OPERATOR_PREPARE_INPUT"))
		if err != nil {
			return err
		}
		output, err := os.OpenFile(os.Getenv("OPERATOR_PREPARE_REPORT"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
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
		tool := operatorprepare.New(db, module.AuthzSnapshotLoader(), iam.NewOperatorRetirementAuthzGateway(iam.NewAuthzAssignmentClient(module.Client()), module.AuthzSnapshotLoader()), func(ctx context.Context, userID int64) error {
			response, err := module.IdentityService().GetUser(ctx, strconv.FormatInt(userID, 10))
			if err != nil {
				return fmt.Errorf("cannot verify target IAM identity")
			}
			if response == nil || response.User == nil || response.User.Id != strconv.FormatInt(userID, 10) || response.User.Status != identityv2.UserStatus_USER_STATUS_ACTIVE {
				return fmt.Errorf("active matching IAM identity required")
			}
			return nil
		})
		var report *operatorprepare.Report
		switch action {
		case "preflight":
			report, err = tool.Preflight(ctx, cmd)
		case "status":
			report, err = tool.Preflight(ctx, cmd)
		case "apply":
			report, err = tool.Apply(ctx, cmd, os.Getenv("OPERATOR_PREPARE_FINGERPRINT"))
		}
		if report == nil {
			report = &operatorprepare.Report{State: "invalid", Command: cmd}
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
		fmt.Printf("operator preparation state=%s; details saved privately\n", report.State)
		if err != nil {
			return fmt.Errorf("preparation stopped; inspect restricted report")
		}
		return nil
	})).Run()
}

func readCommand(path string) (operatorprepare.Command, error) {
	var cmd operatorprepare.Command
	file, err := os.Open(path)
	if err != nil {
		return cmd, fmt.Errorf("preparation input file required")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return cmd, fmt.Errorf("preparation input must be a private regular file smaller than 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65537))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cmd); err != nil {
		return cmd, fmt.Errorf("invalid preparation input")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return cmd, fmt.Errorf("preparation input must contain one JSON object")
	}
	if cmd.OrgID <= 0 || cmd.ActorID <= 0 || cmd.UserID <= 0 || cmd.Name == "" || strings.TrimSpace(cmd.Name) != cmd.Name || cmd.RequestID == "" || len(cmd.RequestID) > 64 || strings.TrimSpace(cmd.RequestID) != cmd.RequestID || strings.TrimSpace(cmd.Reason) != cmd.Reason || cmd.Reason == "" || len([]rune(cmd.Reason)) > 500 {
		return cmd, fmt.Errorf("exact preparation identity, name, request and reason required")
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
		return nil, nil, fmt.Errorf("connect QS MySQL for operator preparation")
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
