package worker

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/config"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/qs-server/pkg/app"
)

// commandDesc 命令描述
const commandDesc = `The QS Worker consumes domain events from message queue and processes
background tasks such as assessment evaluation, notification sending, and statistics collection.

It supports multiple event types:
- assessment.submitted: Triggers evaluation workflow
- assessment.interpreted: Triggers notification and alert
- assessment.failed: Logs and monitors failures`

// NewApp 创建 Worker App
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("QS Worker",
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

		log.Info("Starting qs-worker ...")

		// 根据 options 创建配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 worker
		return Run(cfg)
	}
}
