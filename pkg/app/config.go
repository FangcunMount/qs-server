package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/marmotedu/component-base/pkg/util/homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// configFlagName 配置标志名称。
const configFlagName = "config"

// cfgFile 配置文件。
var cfgFile string

// init 初始化配置标志。
func init() {
	pflag.StringVarP(&cfgFile, "config", "c", cfgFile, "Read configuration from specified `FILE`, "+
		"support JSON, TOML, YAML, HCL, or Java properties formats.")
}

// addConfigFlag 添加配置标志。
// 参数 basename 是 string 类型，表示应用的基名称。
// 参数 fs 是 *pflag.FlagSet 类型，表示标志集。
func addConfigFlag(basename string, fs *pflag.FlagSet) {
	fs.AddFlag(pflag.Lookup(configFlagName))

	viper.AutomaticEnv()
	viper.SetEnvPrefix(strings.Replace(strings.ToUpper(basename), "-", "_", -1))
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	cobra.OnInitialize(func() {
		if cfgFile != "" {
			viper.SetConfigFile(cfgFile)
		} else {
			viper.AddConfigPath(".")

			if names := strings.Split(basename, "-"); len(names) > 1 {
				viper.AddConfigPath(filepath.Join(homedir.HomeDir(), "."+names[0]))
				viper.AddConfigPath(filepath.Join("/etc", names[0]))
			}

			viper.SetConfigName(basename)
		}

		if err := viper.ReadInConfig(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: failed to read configuration file(%s): %v\n", cfgFile, err)
			os.Exit(1)
		}
	})
}
