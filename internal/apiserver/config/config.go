// Copyright 2020 Lingfei Kong <colin404@foxmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package config

import "github.com/yshujie/questionnaire-scale/internal/apiserver/options"

// Config 是 apiserver 的运行配置结构体
type Config struct {
	*options.Options
}

// CreateConfigFromOptions 根据给定的命令行或配置文件选项创建一个运行配置实例
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
