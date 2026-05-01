package scale

import (
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleDomainErrorCodeMapsDomainKindToAPICode(t *testing.T) {
	t.Parallel()

	_, err := domainScale.NewMedicalScale(meta.NewCode(""), "Scale")
	if err == nil {
		t.Fatal("expected domain error")
	}
	if got := scaleDomainErrorCode(err, errorCode.ErrUnknown); got != errorCode.ErrInvalidArgument {
		t.Fatalf("mapped code = %d, want %d", got, errorCode.ErrInvalidArgument)
	}
}
