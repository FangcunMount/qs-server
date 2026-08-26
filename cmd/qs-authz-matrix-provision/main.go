package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/apiserver/maintenance/authzmatrix"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/qs-server/pkg/version"
)

func main() {
	opts := apiserveroptions.NewOptions()
	app.NewApp(
		"Explicitly provision the isolated production IAM AuthZ matrix evaluator",
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

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		iamModule, err := container.NewIAMModuleWithRuntimeOptions(ctx, opts.IAMOptions, container.IAMModuleRuntimeOptions{})
		if err != nil {
			return fmt.Errorf("initialize IAM client: %w", err)
		}
		defer func() { _ = iamModule.Close() }()
		if iamModule.Client() == nil || iamModule.Client().SDK() == nil || iamModule.IdentityService() == nil ||
			iamModule.IdentityService().Raw() == nil || iamModule.ServiceAuthHelper() == nil {
			return fmt.Errorf("IAM provisioning dependencies are unavailable")
		}

		serviceIdentity := iamModule.ServiceAuthHelper().ServiceIdentity().ServiceID
		provisioner := authzmatrix.NewProvisioner(
			iamModule.IdentityService().Raw(), iamModule.Client().SDK().Authz(),
			iamModule.ServiceAuthHelper(), version.Get().GitCommit, serviceIdentity,
		)
		evidence, provisionErr := provisioner.EnsureEvaluator(ctx, os.Getenv("QS_AUTHZ_MATRIX_PROVISION_CONFIRM"))
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(evidence); err != nil {
			return fmt.Errorf("write provisioning evidence: %w", err)
		}
		if provisionErr != nil {
			return fmt.Errorf("provision isolated AuthZ matrix evaluator: %w", provisionErr)
		}
		return nil
	}
}
