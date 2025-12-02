package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FangcunMount/component-base/pkg/util/homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// configFlagName 配置标志名称
const configFlagName = "config"

// cfgFile 配置文件
var cfgFile string

// init 初始化配置标志
func init() {
	pflag.StringVarP(&cfgFile, "config", "c", cfgFile, "Read configuration from specified `FILE`, "+
		"support JSON, TOML, YAML, HCL, or Java properties formats.")
}

// addConfigFlag 添加配置标志
func addConfigFlag(basename string, fs *pflag.FlagSet) {
	// 添加配置标志
	fs.AddFlag(pflag.Lookup(configFlagName))

	// 自动设置环境变量
	viper.AutomaticEnv()
	// 设置环境变量前缀
	viper.SetEnvPrefix(strings.Replace(strings.ToUpper(basename), "-", "_", -1))
	// 设置环境变量键替换
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// 初始化配置
	cobra.OnInitialize(func() {
		// 若指定了配置文件地址，则直接使用
		if cfgFile != "" {
			viper.SetConfigFile(cfgFile)
		} else {
			// 没有指定配置文件地址，则使用当前目录
			viper.AddConfigPath(".")
			// 添加 configs 目录
			viper.AddConfigPath("configs")

			// 如果basename包含多个单词，则添加配置路径
			if names := strings.Split(basename, "-"); len(names) > 1 {
				// 添加用户家目录下的配置路径
				viper.AddConfigPath(filepath.Join(homedir.HomeDir(), "."+names[0]))
				// 添加系统配置路径
				viper.AddConfigPath(filepath.Join("/etc", names[0]))
			}

			// 设置配置文件名
			viper.SetConfigName(basename)
		}

		// 读取配置文件（支持环境变量占位符）
		if err := viper.ReadInConfig(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: failed to read configuration file(%s): %v\n", cfgFile, err)
			os.Exit(1)
		}

		fmt.Printf("%v Config file used: %s\n", progressMessage, viper.ConfigFileUsed())
		printConfigStage("Config loaded from file (before flags/env overrides)")
	})
}

// printConfigStage prints current viper settings with a stage label.
func printConfigStage(stage string) {
	fmt.Printf("%v %s: %+v\n", progressMessage, stage, viper.AllSettings())
}
