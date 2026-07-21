package options

import "testing"

func TestMySQLOptionsDefaultToShanghaiContract(t *testing.T) {
	options := NewMySQLOptions()
	if options.Location != "Asia/Shanghai" || options.SessionTimeZone != "+08:00" {
		t.Fatalf("location=%q session_time_zone=%q", options.Location, options.SessionTimeZone)
	}
	if errs := options.Validate(); len(errs) != 0 {
		t.Fatalf("default mysql options are invalid: %v", errs)
	}
}

func TestMySQLOptionsRejectInvalidTimeContract(t *testing.T) {
	options := NewMySQLOptions()
	options.Location = "Mars/Olympus"
	options.SessionTimeZone = "Shanghai"
	if errs := options.Validate(); len(errs) != 2 {
		t.Fatalf("errors=%v", errs)
	}
}
