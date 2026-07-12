package service

import (
	"errors"
	"strings"
	"testing"
)

func TestInterpretationAutomationUnknownFailureIsSafe(t *testing.T) {
	resp := generateReportFailureResponse(errors.New("mongo password=secret"))
	if strings.Contains(resp.Message, "secret") || resp.FailureCode != "internal_error" || resp.FailureKind != "internal" {
		t.Fatalf("unsafe failure response: %#v", resp)
	}
}
