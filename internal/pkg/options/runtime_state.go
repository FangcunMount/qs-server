package options

import "fmt"

// RuntimeStateOptions contains Redis-backed operational state that is not a
// read-through business cache.
type RuntimeStateOptions struct {
	ReportStatus *ReportStatusOptions `json:"report_status" mapstructure:"report_status"`
}

func NewRuntimeStateOptions() *RuntimeStateOptions {
	return &RuntimeStateOptions{ReportStatus: NewReportStatusOptions()}
}

func (o *RuntimeStateOptions) Validate(prefix string) []error {
	if o == nil {
		return []error{fmt.Errorf("%s cannot be nil", prefix)}
	}
	if o.ReportStatus == nil {
		return []error{fmt.Errorf("%s.report_status cannot be nil", prefix)}
	}
	if o.ReportStatus.TTLSeconds <= 0 {
		return []error{fmt.Errorf("%s.report_status.ttl_seconds must be greater than 0", prefix)}
	}
	return nil
}

func RuntimeStateRawSchema() FieldSchema {
	leaf := FieldSchema(nil)
	return FieldSchema{"report_status": {"ttl_seconds": leaf}}
}
