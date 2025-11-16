package evaluation

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/config"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/options"
	"github.com/FangcunMount/qs-server/pkg/app"
)

// commandDesc 命令描述
const commandDesc = `The Questionnaire Evaluation Server provides automated scoring and report generation.
It subscribes to "raw answer sheet saved" events, performs scoring calculations, 
generates interpretation reports, and saves them to the database.`

// NewApp 创建 App
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("Questionnaire Evaluation Server",
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

		log.Info("Starting evaluation-server ...")

		// 打印配置信息
		log.Infof("Server mode: %s", opts.GenericServerRunOptions.Mode)
		log.Infof("Health check enabled: %v", opts.GenericServerRunOptions.Healthz)
		log.Infof("Message queue endpoint: %s", opts.MessageQueue.Endpoint)
		log.Infof("GRPC client endpoint: %s", opts.GRPCClient.Endpoint)

		// 根据 options 创建 app 配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 app
		return Run(cfg)
	}
}
