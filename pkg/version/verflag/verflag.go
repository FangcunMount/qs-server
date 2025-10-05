package verflag

import (
	"fmt"
	"os"
	"strconv"

	flag "github.com/spf13/pflag"

	"github.com/fangcun-mount/qs-server/pkg/version"
)

// versionValue 版本值
type versionValue int

// 定义一些常量
const (
	VersionFalse versionValue = 0
	VersionTrue  versionValue = 1
	VersionRaw   versionValue = 2
)

// strRawVersion 原始版本
const strRawVersion string = "raw"

// IsBoolFlag 是否为布尔标志
func (v *versionValue) IsBoolFlag() bool {
	return true
}

// Get 获取版本值
func (v *versionValue) Get() interface{} {
	return v
}

// Set 设置版本值
func (v *versionValue) Set(s string) error {
	if s == strRawVersion {
		*v = VersionRaw
		return nil
	}
	boolVal, err := strconv.ParseBool(s)
	if boolVal {
		*v = VersionTrue
	} else {
		*v = VersionFalse
	}
	return err
}

// String 返回版本值字符串
func (v *versionValue) String() string {
	if *v == VersionRaw {
		return strRawVersion
	}
	return fmt.Sprintf("%v", bool(*v == VersionTrue))
}

// Type 返回标志类型
func (v *versionValue) Type() string {
	return "version"
}

// VersionVar 定义一个标志
func VersionVar(p *versionValue, name string, value versionValue, usage string) {
	*p = value
	flag.Var(p, name, usage)
	// "--version" will be treated as "--version=true"
	flag.Lookup(name).NoOptDefVal = "true"
}

// Version 包装 VersionVar 函数
func Version(name string, value versionValue, usage string) *versionValue {
	p := new(versionValue)
	VersionVar(p, name, value, usage)
	return p
}

const versionFlagName = "version"

var versionFlag = Version(versionFlagName, VersionFalse, "Print version information and quit.")

// AddFlags 注册此包的标志到任意 FlagSets，使得它们指向全局标志
func AddFlags(fs *flag.FlagSet) {
	fs.AddFlag(flag.Lookup(versionFlagName))
}

// PrintAndExitIfRequested 检查 -version 标志是否被传递，如果传递，则打印版本并退出
func PrintAndExitIfRequested() {
	if *versionFlag == VersionRaw {
		fmt.Printf("%#v\n", version.Get())
		os.Exit(0)
	} else if *versionFlag == VersionTrue {
		fmt.Printf("%s\n", version.Get())
		os.Exit(0)
	}
}
