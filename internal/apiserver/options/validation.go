package options

import "fmt"

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	// 验证日志配置
	errs = append(errs, o.Log.Validate()...)

	// 验证服务器配置
	if o.Server.Mode != "release" && o.Server.Mode != "debug" && o.Server.Mode != "test" {
		errs = append(errs, fmt.Errorf("invalid server mode: %s", o.Server.Mode))
	}

	if o.Server.MaxPingCount <= 0 {
		errs = append(errs, fmt.Errorf("max-ping-count must be greater than 0"))
	}

	return errs
}
