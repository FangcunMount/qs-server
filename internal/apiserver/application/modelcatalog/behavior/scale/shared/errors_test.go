package shared

import (
	"testing"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleDomainErrorCodeMapsDomainKindToAPICode(t *testing.T) {
	t.Parallel()

	_, err := scaledefinition.NewMedicalScale(meta.NewCode(""), "Scale")
	if err == nil {
		t.Fatal("expected domain error")
	}
	if got := ScaleDomainErrorCode(err, errorCode.ErrUnknown); got != errorCode.ErrInvalidArgument {
		t.Fatalf("mapped code = %d, want %d", got, errorCode.ErrInvalidArgument)
	}
}
