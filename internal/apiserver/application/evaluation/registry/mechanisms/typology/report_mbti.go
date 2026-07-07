// 仅用于表征: 类型学 报告构建器 辅助函数，用于 V1 契约测试。
package typology

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
)

// NewMBTIReportBuilder 是表征 辅助函数，用于 类型学 reports。
func NewMBTIReportBuilder() interpretationreporting.ReportBuilder {
	builder, err := NewConfiguredReportBuilder()
	if err != nil {
		panic(err)
	}
	return builder
}
