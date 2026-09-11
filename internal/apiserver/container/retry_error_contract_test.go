package container

import (
	"fmt"
	baseerrors "github.com/FangcunMount/component-base/pkg/errors"
	operator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"testing"
)

func TestRetryStateConflictResponse(t *testing.T) {
	err := normalizeGovernedRetryError(fmt.Errorf("completed assessment: %w", operator.ErrRetryStateConflict))
	c := baseerrors.ParseCoder(err)
	if c.Code() != code.ErrConflict || c.HTTPStatus() != 409 {
		t.Fatalf("retry state conflict: code=%d status=%d", c.Code(), c.HTTPStatus())
	}
}
func TestRetryUnexpectedErrorRemainsServerError(t *testing.T) {
	err := fmt.Errorf("storage unavailable")
	if normalizeGovernedRetryError(err) != err {
		t.Fatal("unexpected errors must not become state conflicts")
	}
}
