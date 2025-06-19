package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// App 应用
type App struct {
	basename string
	name     string
	cmd      *cobra.Command
	args     cobra.PositionalArgs
}

// Option 应用选项
type Option func(*App)

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

	a.cmd = cmd
}
