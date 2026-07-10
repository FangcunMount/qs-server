package shared

import (
	"fmt"
	"testing"

	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestScaleDomainErrorCodeUsesFallback(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("invalid scale")
	if got := ScaleDomainErrorCode(err, errorCode.ErrUnknown); got != errorCode.ErrUnknown {
		t.Fatalf("mapped code = %d, want %d", got, errorCode.ErrUnknown)
	}
}
