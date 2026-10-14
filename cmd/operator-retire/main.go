package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/operatorretire"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: operator-retire selected-preflight|selected-apply|selected-verify|orphan-preflight|orphan-apply|orphan-verify [QS config flags]; required environment: OPERATOR_RETIRE_LAYOUT=final, OPERATOR_RETIRE_INPUT, OPERATOR_RETIRE_REPORT; apply also requires OPERATOR_RETIRE_FINGERPRINT, OPERATOR_RETIRE_MAINTENANCE=true")
		os.Exit(1)
	}
	action := os.Args[1]
	switch action {
	case "preflight", "apply", "status", "verify":
		fmt.Fprintln(os.Stderr, "historical batch retirement actions are retired; archived tools are restricted to isolated historical databases")
		os.Exit(1)
	case "selected-preflight", "selected-apply", "selected-verify", "orphan-preflight", "orphan-apply", "orphan-verify":
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
		conn, err := maintenance.OpenMySQL(opts.MySQLOptions, "operator retirement")
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
		if strings.HasPrefix(action, "orphan-") {
			return runOrphan(ctx, tool, action, output)
		}
		return runSelected(ctx, tool, action, output)
	})).Run()
}
