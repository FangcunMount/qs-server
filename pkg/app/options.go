// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package app

import (
	cliflag "github.com/marmotedu/component-base/pkg/cli/flag"
)

// CliOptions abstracts configuration options for reading parameters from the
// command line.
// CliOptions 抽象了从命令行读取参数的选项。
type CliOptions interface {
	// Flags 返回一个 NamedFlagSets 对象，包含所有命令行参数。
	Flags() (fss cliflag.NamedFlagSets)
	// Validate 验证命令行参数。
	Validate() []error
}

// ConfigurableOptions 抽象了从配置文件读取参数的选项。
type ConfigurableOptions interface {
	// ApplyFlags 解析命令行或配置文件的参数到选项实例。
	ApplyFlags() []error
}

// CompleteableOptions 抽象了可以完成的选项。
type CompleteableOptions interface {
	Complete() error
}

// PrintableOptions 抽象了可以打印的选项。
type PrintableOptions interface {
	String() string
}
