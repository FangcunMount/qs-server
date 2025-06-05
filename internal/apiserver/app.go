// Package apiserver does all the work necessary to create a qs APIServer.
package apiserver

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/config"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/options"
	"github.com/yshujie/questionnaire-scale/pkg/app"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// commandDesc 命令描述
const commandDesc = `The QS API server validates and configures data
for the questionnaire scale service.`

// NewApp 创建一个 app 对象
func NewApp(basename string) *app.App {
	opts := options.NewOptions()
	application := app.NewApp("QS API Server",
		basename,
		app.WithOptions(opts),
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithRunFunc(run(opts)),
	)

	return application
}

// run 运行 apiserver
func run(opts *options.Options) app.RunFunc {
	return func(basename string) error {
		// 初始化日志
		log.Init(opts.Log)
		defer log.Flush()

		// 创建配置
		cfg, err := config.CreateConfigFromOptions(opts)
		if err != nil {
			return err
		}

		// 运行 apiserver
		return Run(cfg)
	}
}
