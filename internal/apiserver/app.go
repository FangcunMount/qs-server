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
		log.Init(log.NewOptions())
		defer log.Flush()

		log.Info("Starting questionnaire-scale ...")

		return nil
	}
}
