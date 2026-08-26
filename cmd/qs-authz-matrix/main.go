package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/authzmatrix"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/qs-server/pkg/version"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	opts := apiserveroptions.NewOptions()
	app.NewApp(
		"Read-only production IAM AuthZ v3 matrix verifier",
		// Keep the production environment-variable prefix identical to the
		// online process. The verifier runs in the same container configuration.
		"qs-apiserver",
		app.WithDefaultValidArgs(),
		app.WithOptions(opts),
		app.WithRunFunc(run(opts)),
	).Run()
}

func run(opts *apiserveroptions.Options) app.RunFunc {
	return func(_ string) error {
		log.Init(opts.Log)
		defer log.Flush()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		mysqlConnection, sqlDB, err := openReadOnlySource(opts)
		if err != nil {
			return err
		}
		defer func() { _ = mysqlConnection.Close() }()

		iamModule, err := container.NewIAMModuleWithRuntimeOptions(ctx, opts.IAMOptions, container.IAMModuleRuntimeOptions{})
		if err != nil {
			return fmt.Errorf("initialize IAM v3 client: %w", err)
		}
		defer func() { _ = iamModule.Close() }()
		if iamModule.AuthzSnapshotLoader() == nil || iamModule.ObjectAuthorizationChecker() == nil ||
			iamModule.ServiceAuthHelper() == nil || iamModule.IdentityService() == nil || iamModule.IdentityService().Raw() == nil {
			return fmt.Errorf("IAM v3 matrix dependencies are unavailable")
		}

		serviceIdentity := iamModule.ServiceAuthHelper().ServiceIdentity().ServiceID
		runner := authzmatrix.NewRunner(
			authzmatrix.NewStableSubjectSource(sqlDB, authzmatrix.NewIAMSyntheticSubjectDirectory(
				iamModule.IdentityService().Raw(), iamModule.ServiceAuthHelper(),
			), iamModule.AuthzSnapshotLoader()),
			iamModule.AuthzSnapshotLoader(),
			iamModule.ObjectAuthorizationChecker(),
			version.Get().GitCommit,
			serviceIdentity,
		)
		evidence, runErr := runner.Run(ctx)
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(evidence); err != nil {
			return fmt.Errorf("write AuthZ matrix evidence: %w", err)
		}
		if runErr != nil {
			return fmt.Errorf("production AuthZ matrix failed: %w", runErr)
		}
		return nil
	}
}

func openReadOnlySource(opts *apiserveroptions.Options) (*database.MySQLConnection, *sql.DB, error) {
	if opts == nil || opts.MySQLOptions == nil {
		return nil, nil, fmt.Errorf("qs MySQL options are required")
	}
	configured := opts.MySQLOptions
	connection := database.NewMySQLConnection(&database.MySQLConfig{
		Host: configured.Host, Username: configured.Username, Password: configured.Password, Database: configured.Database,
		MaxIdleConnections: 1, MaxOpenConnections: 2, MaxConnectionLifeTime: configured.MaxConnectionLifeTime,
		LogLevel: configured.LogLevel, Location: configured.Location, SessionTimeZone: configured.SessionTimeZone,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err := connection.Connect(); err != nil {
		return nil, nil, fmt.Errorf("connect qs MySQL for read-only subject selection: %w", err)
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
