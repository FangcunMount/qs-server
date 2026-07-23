package queryerror

import (
	"errors"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestMapReadErrorKeepsNotFoundConsistencyAndDependencyDistinct(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "not found", err: interpretationreadmodel.ErrReportNotFound, code: code.ErrInterpretReportNotFound},
		{name: "dangling", err: &interpretationreadmodel.CatalogDanglingSourceError{AssessmentID: 1}, code: code.ErrInterpretReportConsistency},
		{name: "association mismatch", err: &interpretationreadmodel.CatalogSourceAssociationMismatchError{AssessmentID: 1}, code: code.ErrInterpretReportConsistency},
		{name: "dependency", err: errors.New("mongo unavailable"), code: code.ErrDatabase},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MapReadError(tc.err)
			if !cberrors.IsCode(got, tc.code) {
				t.Fatalf("MapReadError() = %v, want code %d", got, tc.code)
			}
			if tc.code == code.ErrInterpretReportConsistency && got.Error() != "report temporarily unavailable" {
				t.Fatalf("consistency error = %q", got.Error())
			}
		})
	}
}
