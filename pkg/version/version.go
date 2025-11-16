package version

import (
	"fmt"
	"runtime"

	"github.com/gosuri/uitable"

	"github.com/FangcunMount/component-base/pkg/json"
)

var (
	// GitVersion 版本号
	GitVersion = "v0.0.0-master+$Format:%h$"
	// BuildDate 构建日期
	BuildDate = "1970-01-01T00:00:00Z"
	// GitCommit 提交哈希
	GitCommit = "$Format:%H$"
	// GitTreeState 树状态
	GitTreeState = ""
)

// Info 版本信息
type Info struct {
	GitVersion   string `json:"gitVersion"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
}

// String 返回版本信息字符串
func (info Info) String() string {
	if s, err := info.Text(); err == nil {
		return string(s)
	}

	return info.GitVersion
}

// ToJSON 返回版本信息JSON
func (info Info) ToJSON() string {
	s, _ := json.Marshal(info)

	return string(s)
}

// Text 返回版本信息文本
func (info Info) Text() ([]byte, error) {
	table := uitable.New()
	table.RightAlign(0)
	table.MaxColWidth = 80
	table.Separator = " "
	table.AddRow("gitVersion:", info.GitVersion)
	table.AddRow("gitCommit:", info.GitCommit)
	table.AddRow("gitTreeState:", info.GitTreeState)
	table.AddRow("buildDate:", info.BuildDate)
	table.AddRow("goVersion:", info.GoVersion)
	table.AddRow("compiler:", info.Compiler)
	table.AddRow("platform:", info.Platform)

	return table.Bytes(), nil
}

// Get 返回版本信息
func Get() Info {
	return Info{
		GitVersion:   GitVersion,
		GitCommit:    GitCommit,
		GitTreeState: GitTreeState,
		BuildDate:    BuildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
