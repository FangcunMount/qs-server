package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cliflag "github.com/yshujie/questionnaire-scale/pkg/flag"
)

// App 应用
type App struct {
	basename string
	name     string
	options  CliOptions
	cmd      *cobra.Command
	args     cobra.PositionalArgs
	commands []*Command
}

// Option 应用选项
type Option func(*App)

// WithOptions 打开应用程序的函数，从命令行或配置文件中读取参数
func WithOptions(opt CliOptions) Option {
	return func(a *App) {
		a.options = opt
	}
}

// WithValidArgs 设置 args
func WithValidArgs(args cobra.PositionalArgs) Option {
	return func(a *App) {
		a.args = args
	}
}

// WithDefaultValidArgs 设置默认的 args
func WithDefaultValidArgs() Option {
	return func(a *App) {
		a.args = func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}

			return nil
		}
	}
}

// NewApp 创建应用
func NewApp(name string, basename string, opts ...Option) *App {
	// 创建 App
	a := &App{
		name:     name,
		basename: basename,
	}
	// 设置应用选项
	for _, opt := range opts {
		opt(a)
	}

	// 构建命令
	a.buildCommand()

	// 返回 app
	return a
}

// buildCommand 构建命令
func (a *App) buildCommand() {
	// 使用 cobra 创建命令
	cmd := &cobra.Command{
		Use:           FormatBaseName(a.basename),
		Short:         a.name,
		Long:          a.name,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          a.args,
	}

	// 设置输出和错误输出
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	// 设置排序
	cmd.Flags().SortFlags = true

	// 初始化命令行参数
	cliflag.InitFlags(cmd.Flags())

	// 如果命令不为空，则添加命令
	if len(a.commands) > 0 {
		// 添加命令
		for _, command := range a.commands {
			cmd.AddCommand(command.cobraCommand())
		}
		// 设置帮助命令
		cmd.SetHelpCommand(helpCommand(FormatBaseName(a.basename)))
	}

	a.cmd = cmd
}
