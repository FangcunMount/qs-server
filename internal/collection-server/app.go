package collection

import (
	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/pkg/app"
)

// commandDesc 命令描述
const commandDesc = `The Questionnaire Collection Server provides REST API for questionnaire collection system (mini-program).
It validates questionnaire submissions and communicates with apiserver via gRPC for data operations.`

// NewApp 创建 App
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("Questionnaire Collection Server",
		basename,
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithOptions(opts),
		app.WithRunFunc(run(opts)),
	)

	return application
}

func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		// 初始化日志
		log.Init(opts.Log)
		defer log.Flush()

		log.Infof("Starting collection-server ... (mode=%s, healthz=%v)", opts.GenericServerRunOptions.Mode, opts.GenericServerRunOptions.Healthz)
		log.Infof("HTTP bind: %s:%d, HTTPS bind: %s:%d, gRPC endpoint: %s",
			opts.InsecureServing.BindAddress, opts.InsecureServing.BindPort,
			opts.SecureServing.BindAddress, opts.SecureServing.BindPort,
			opts.GRPCClient.Endpoint)

		// 打印安全与并发配置
		log.Infof("TLS cert: %s, key: %s", opts.SecureServing.TLS.CertFile, opts.SecureServing.TLS.KeyFile)
		log.Infof("Concurrency: max=%d, JWT expiry(h): %d", opts.Concurrency.MaxConcurrency, opts.JWT.TokenDuration)

		// 根据 options 创建 app 配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 app
		return Run(cfg)
	}
}
