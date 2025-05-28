package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	cliflag "github.com/marmotedu/component-base/pkg/cli/flag"
	"github.com/marmotedu/component-base/pkg/cli/globalflag"
	"github.com/marmotedu/component-base/pkg/term"
	"github.com/marmotedu/component-base/pkg/version"
	"github.com/marmotedu/component-base/pkg/version/verflag"
	"github.com/marmotedu/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// progressMessage 进度消息。
var progressMessage = color.GreenString("==>")

// App 定义应用
type App struct {
	basename    string
	name        string
	description string
	options     CliOptions
	runFunc     RunFunc
	silence     bool
	noVersion   bool
	noConfig    bool
	commands    []*Command
	args        cobra.PositionalArgs
	cmd         *cobra.Command
}

// Option 定义一个函数，用于设置应用的选项。
type Option func(*App)

// WithOptions 设置应用的选项。
// 参数 options 是 CliOptions 类型，表示应用的选项。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithOptions(options CliOptions) Option {
	return func(app *App) {
		app.options = options
	}
}

// RunFunc 定义了应用的运行函数。
type RunFunc func(basename string) error

// WithRunFunc 设置应用的运行函数。
// 参数 runFunc 是 RunFunc 类型，表示应用的运行函数。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithRunFunc(runFunc RunFunc) Option {
	return func(app *App) {
		app.runFunc = runFunc
	}
}

// WithDescription 设置应用的描述。
// 参数 description 是 string 类型，表示应用的描述。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithDescription(description string) Option {
	return func(app *App) {
		app.description = description
	}
}

// WithSilence 设置应用的静默模式。
// 参数 silence 是 bool 类型，表示应用的静默模式。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithSilence(silence bool) Option {
	return func(app *App) {
		app.silence = silence
	}
}

// WithNoVersion 设置应用的版本。
// 参数 noVersion 是 bool 类型，表示应用的版本。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithNoVersion(noVersion bool) Option {
	return func(app *App) {
		app.noVersion = noVersion
	}
}

// WithNoConfig 设置应用的配置。
// 参数 noConfig 是 bool 类型，表示应用的配置。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithNoConfig(noConfig bool) Option {
	return func(app *App) {
		app.noConfig = noConfig
	}
}

// WithValidArgs 设置应用的合法参数。
// 参数 args 是 cobra.PositionalArgs 类型，表示应用的合法参数。
// 返回值是一个 Option 类型，表示一个应用选项。
func WithValidArgs(args cobra.PositionalArgs) Option {
	return func(a *App) {
		a.args = args
	}
}

// WithDefaultValidArgs 设置应用的默认合法参数。
// 返回值是一个 Option 类型，表示一个应用选项。
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

// NewApp 创建一个应用。
// 参数 name 是 string 类型，表示应用的名称。
// 参数 basename 是 string 类型，表示应用的基名称。
// 参数 options 是 Option 类型，表示应用的选项。
// 返回值是一个 App 类型，表示一个应用。
func NewApp(name, basename string, options ...Option) *App {
	// 创建应用
	app := &App{
		name:     name,
		basename: basename,
	}

	// 设置应用选项
	for _, option := range options {
		option(app)
	}

	// 构建应用命令
	app.buildCommand()

	return app
}

// buildCommand 构建应用命令。
func (a *App) buildCommand() {
	// 使用 cobra 创建命令
	cmd := cobra.Command{
		Use:   FormatBaseName(a.basename),
		Short: a.name,
		Long:  a.description,
		// stop printing usage when the command errors
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          a.args,
	}
	// 设置输出和错误输出
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.Flags().SortFlags = true

	// 初始化命令行标志
	cliflag.InitFlags(cmd.Flags())

	// 添加子命令
	if len(a.commands) > 0 {
		for _, command := range a.commands {
			cmd.AddCommand(command.cobraCommand())
		}
		cmd.SetHelpCommand(helpCommand(FormatBaseName(a.basename)))
	}
	// 设置运行函数
	if a.runFunc != nil {
		cmd.RunE = a.runCommand
	}

	// 添加命令行标志
	var namedFlagSets cliflag.NamedFlagSets
	if a.options != nil {
		namedFlagSets = a.options.Flags()
		fs := cmd.Flags()
		for _, f := range namedFlagSets.FlagSets {
			fs.AddFlagSet(f)
		}
	}

	// 添加版本标志
	if !a.noVersion {
		verflag.AddFlags(namedFlagSets.FlagSet("global"))
	}
	// 添加配置标志
	if !a.noConfig {
		addConfigFlag(a.basename, namedFlagSets.FlagSet("global"))
	}
	// 添加全局标志
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())
	// 添加新的全局标志到 cmd FlagSet
	cmd.Flags().AddFlagSet(namedFlagSets.FlagSet("global"))

	// 添加命令模板
	addCmdTemplate(&cmd, namedFlagSets)
	a.cmd = &cmd
}

// Run 启动应用
func (a *App) Run() {
	if err := a.cmd.Execute(); err != nil {
		fmt.Printf("%v %v\n", color.RedString("Error:"), err)
		os.Exit(1)
	}
}

// Command 返回应用的 cobra 命令实例
func (a *App) Command() *cobra.Command {
	return a.cmd
}

// runCommand 运行命令
func (a *App) runCommand(cmd *cobra.Command, args []string) error {
	printWorkingDir()
	cliflag.PrintFlags(cmd.Flags())
	if !a.noVersion {
		// 显示应用版本信息
		verflag.PrintAndExitIfRequested()
	}

	// 绑定配置标志
	if !a.noConfig {
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			return err
		}

		if err := viper.Unmarshal(a.options); err != nil {
			return err
		}
	}

	// 打印应用信息
	if !a.silence {
		log.Infof("%v Starting %s ...", progressMessage, a.name)
		if !a.noVersion {
			log.Infof("%v Version: `%s`", progressMessage, version.Get().ToJSON())
		}
		if !a.noConfig {
			log.Infof("%v Config file used: `%s`", progressMessage, viper.ConfigFileUsed())
		}
	}
	// 应用选项
	if a.options != nil {
		if err := a.applyOptionRules(); err != nil {
			return err
		}
	}
	// 运行应用
	if a.runFunc != nil {
		return a.runFunc(a.basename)
	}

	return nil
}

// applyOptionRules 应用选项规则
func (a *App) applyOptionRules() error {
	if completeableOptions, ok := a.options.(CompleteableOptions); ok {
		if err := completeableOptions.Complete(); err != nil {
			return err
		}
	}

	if errs := a.options.Validate(); len(errs) != 0 {
		return errors.NewAggregate(errs)
	}

	if printableOptions, ok := a.options.(PrintableOptions); ok && !a.silence {
		log.Infof("%v Config: `%s`", progressMessage, printableOptions.String())
	}

	return nil
}

// printWorkingDir 打印工作目录
func printWorkingDir() {
	wd, _ := os.Getwd()
	log.Infof("%v WorkingDir: %s", progressMessage, wd)
}

// addCmdTemplate 添加命令模板
func addCmdTemplate(cmd *cobra.Command, namedFlagSets cliflag.NamedFlagSets) {
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)

		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})
}
