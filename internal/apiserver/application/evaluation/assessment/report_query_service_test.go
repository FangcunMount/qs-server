package assessment

import (
	"context"
	"testing"

	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestReportQueryServiceExportPDFReturnsUnsupported(t *testing.T) {
	svc := NewReportQueryService(nil)

	_, err := svc.ExportPDF(context.Background(), 1001)
	if err == nil {
		t.Fatalf("expected unsupported export error")
	}
	if !cbErrors.IsCode(err, code.ErrUnsupportedOperation) {
		t.Fatalf("expected ErrUnsupportedOperation, got %v", err)
	}
}
