package apiserver

import (
	"github.com/yshujie/questionnaire-scale/pkg/app"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// commandDesc 命令描述
const commandDesc = `The Questionnaire Scale API server validates and configures data
for the api objects which include users, policies, secrets, and
others. The API Server services REST operations to do the api objects management.`

// NewApp 创建 App
func NewApp(basename string) *app.App {
	application := app.NewApp("Questionnaire Scale API Server",
		basename,
		app.WithDescription(commandDesc),
		app.WithDefaultValidArgs(),
		app.WithOptions(NewOptions()),
		app.WithRunFunc(run()),
	)

	return application
}

func run() app.RunFunc {
	return func(basename string) error {
		// 获取配置选项
		options := NewOptions()

		// 初始化日志
		log.Init(options.Log)
		defer log.Flush()

		log.Info("Starting questionnaire-scale ...")

		// 打印配置信息
		log.Infof("Server mode: %s", options.Server.Mode)
		log.Infof("Health check enabled: %v", options.Server.Healthz)

		return nil
	}
}
