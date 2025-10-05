package config

import "github.com/fangcun-mount/qs-server/internal/evaluation-server/options"

// Config 运行配置结构体
type Config struct {
	*options.Options
}

// CreateConfigFromOptions 根据给定的命令行或配置文件选项创建运行配置实例
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{opts}, nil
}
