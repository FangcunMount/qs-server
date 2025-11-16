package collection

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/config"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/qs-server/pkg/log"
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

		log.Info("Starting collection-server ...")

		// 打印配置信息
		log.Infof("Server mode: %s", opts.GenericServerRunOptions.Mode)
		log.Infof("Health check enabled: %v", opts.GenericServerRunOptions.Healthz)

		// 根据 options 创建 app 配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 app
		return Run(cfg)
	}
}
