package app

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Command 命令
type Command struct {
	usage    string
	desc     string
	options  CliOptions
	commands []*Command
	runFunc  RunCommandFunc
}

// CommandOption 命令选项
type CommandOption func(*Command)

// NewCommand 创建命令
func NewCommand(usage string, desc string, opts ...CommandOption) *Command {
	// 创建命令
	c := &Command{
		usage: usage,
		desc:  desc,
	}

	// 应用选项
	for _, o := range opts {
		o(c)
	}

	// 返回命令
	return c
}

// RunCommandFunc 定义应用程序的命令启动回调函数
type RunCommandFunc func(args []string) error

// FormatBaseName 格式化基础名称
func FormatBaseName(basename string) string {
	// 根据操作系统，将名称转换为小写，并去除可执行文件后缀
	if runtime.GOOS == "windows" {
		basename = strings.ToLower(basename)
		basename = strings.TrimSuffix(basename, ".exe")
	}

	return basename
}

// cobraCommand 创建 cobra 命令
func (c *Command) cobraCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   c.usage,
		Short: c.desc,
	}
	cmd.SetOutput(os.Stdout)
	cmd.Flags().SortFlags = false
	if len(c.commands) > 0 {
		for _, command := range c.commands {
			cmd.AddCommand(command.cobraCommand())
		}
	}
	if c.runFunc != nil {
		cmd.Run = c.runCommand
	}
	if c.options != nil {
		for _, f := range c.options.Flags().FlagSets {
			cmd.Flags().AddFlagSet(f)
		}
		// c.options.AddFlags(cmd.Flags())
	}
	addHelpCommandFlag(c.usage, cmd.Flags())

	return cmd
}

// runCommand 运行命令
func (c *Command) runCommand(cmd *cobra.Command, args []string) {
	if c.runFunc != nil {
		if err := c.runFunc(args); err != nil {
			fmt.Printf("%v %v\n", color.RedString("Error:"), err)
			os.Exit(1)
		}
	}
}
