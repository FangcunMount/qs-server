package app

import (
	cliflag "github.com/yshujie/questionnaire-scale/pkg/flag"
)

// CliOptions 命令行选项
type CliOptions interface {
	// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
	Flags() (fss cliflag.NamedFlagSets)
	// Validate 验证命令行参数
	Validate() []error
}
