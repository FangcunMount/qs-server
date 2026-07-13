package app

import (
	"context"

	cliflag "github.com/FangcunMount/qs-server/pkg/flag"
)

type RawSettings struct {
	Values map[string]any
	Source string
}

// RawSettingsSource re-reads the startup configuration without mutating the
// process-global Viper instance or the already-decoded Options value.
type RawSettingsSource interface {
	Read(context.Context) (RawSettings, error)
}

type RawSettingsSourceAware interface {
	SetRawSettingsSource(RawSettingsSource)
}

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

// RawSettingsValidatable optionally validates the raw configuration tree before
// Viper decodes it into typed options.
type RawSettingsValidatable interface {
	ValidateRawSettings(map[string]any) error
}
