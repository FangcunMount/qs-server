package apperrors

import (
	"errors"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestApplicationErrorMapperPreservesAPICodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		code int
	}{
		{name: "invalid argument", err: InvalidArgument("bad request"), code: errorCode.ErrInvalidArgument},
		{name: "module not configured", err: ModuleNotConfigured("missing dependency"), code: errorCode.ErrModuleInitializationFailed},
		{name: "database", err: Database(errors.New("db"), "database failed"), code: errorCode.ErrDatabase},
		{name: "assessment not found", err: AssessmentNotFound(errors.New("missing"), "assessment missing"), code: errorCode.ErrAssessmentNotFound},
		{name: "assessment invalid status", err: AssessmentInvalidStatus("invalid status"), code: errorCode.ErrAssessmentInvalidStatus},
		{name: "interpret report not found", err: InterpretReportNotFound(errors.New("missing"), "report missing"), code: errorCode.ErrInterpretReportNotFound},
		{name: "unsupported", err: UnsupportedOperation("unsupported"), code: errorCode.ErrUnsupportedOperation},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !cberrors.IsCode(tc.err, tc.code) {
				t.Fatalf("error code = %d, want %d", cberrors.ParseCoder(tc.err).Code(), tc.code)
			}
		})
	}
}
