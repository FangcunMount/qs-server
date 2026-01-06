package options

import "fmt"

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.MySQLOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	if o.RateLimit != nil && o.RateLimit.Enabled {
		if o.RateLimit.SubmitGlobalQPS <= 0 || o.RateLimit.SubmitGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_* must be greater than 0"))
		}
		if o.RateLimit.SubmitUserQPS <= 0 || o.RateLimit.SubmitUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_user_* must be greater than 0"))
		}
		if o.RateLimit.QueryGlobalQPS <= 0 || o.RateLimit.QueryGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_* must be greater than 0"))
		}
		if o.RateLimit.QueryUserQPS <= 0 || o.RateLimit.QueryUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_user_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportGlobalQPS <= 0 || o.RateLimit.WaitReportGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportUserQPS <= 0 || o.RateLimit.WaitReportUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_user_* must be greater than 0"))
		}
	}

	return errs
}
