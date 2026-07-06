package evaluationinput

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDecodePublishedTypologyModelRejectsDraftPayload(t *testing.T) {
	_, err := decodePublishedTypologyModel(&domain.PublishedModelSnapshot{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       []byte(`{"code":"personality_draft","status":"draft","algorithm":"mbti"}`),
	})
	if err == nil {
		t.Fatal("decodePublishedTypologyModel() error = nil, want draft rejection")
	}
}
