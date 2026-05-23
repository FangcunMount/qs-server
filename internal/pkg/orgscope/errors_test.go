package orgscope

import (
	"errors"
	"net/http"
	"testing"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestHTTPStatusForResolveError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "mismatch", err: ErrMismatch, want: http.StatusForbidden},
		{name: "unresolved", err: ErrUnresolved, want: http.StatusUnauthorized},
		{
			name: "permission denied from resolver",
			err:  pkgerrors.WithCode(code.ErrPermissionDenied, "operator not found"),
			want: http.StatusForbidden,
		},
		{
			name: "invalid argument from resolver",
			err:  pkgerrors.WithCode(code.ErrInvalidArgument, "multiple active organizations"),
			want: http.StatusBadRequest,
		},
		{name: "wrapped mismatch", err: errors.Join(ErrMismatch, errors.New("detail")), want: http.StatusForbidden},
		{name: "unknown", err: errors.New("boom"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HTTPStatusForResolveError(tt.err); got != tt.want {
				t.Fatalf("HTTPStatusForResolveError() = %d, want %d", got, tt.want)
			}
		})
	}
}
