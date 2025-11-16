package flag

import (
	goflag "flag"
	"strings"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/spf13/pflag"
)

// InitFlags 初始化
func InitFlags(flags *pflag.FlagSet) {
	// 设置规范化函数
	flags.SetNormalizeFunc(WordSepNormalizeFunc)
	// 添加 goflag 命令行标志
	flags.AddGoFlagSet(goflag.CommandLine)
}

// WordSepNormalizeFunc 单词分隔符规范化函数
func WordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
	}
	return pflag.NormalizedName(name)
}

// WarnWordSepNormalizeFunc 警告包含 "_" 分隔符的标志
// 当命令行参数中带有 "_" 时输出警告，并自动转换为 "-"
func WarnWordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	if strings.Contains(name, "_") {
		nname := strings.ReplaceAll(name, "_", "-")
		log.Warnf("%s is DEPRECATED and will be removed in a future version. Use %s instead.", name, nname)

		return pflag.NormalizedName(nname)
	}
	return pflag.NormalizedName(name)
}

// PrintFlags 打印标志
// 可以打印所有的命令行参数
func PrintFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		log.Debugf("FLAG: --%s=%q", flag.Name, flag.Value)
	})
}
