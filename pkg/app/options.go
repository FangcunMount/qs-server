package app

import (
	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
)

// CliOptions 命令行选项
type CliOptions interface {
	// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数
	Flags() (fss cliflag.NamedFlagSets)
	// Validate 验证命令行参数
	Validate() []error
}

// CompleteableOptions 抽象选项，可以被完成
type CompleteableOptions interface {
	Complete() error
}

// PrintableOptions 抽象选项，可以被打印
type PrintableOptions interface {
	String() string
}
