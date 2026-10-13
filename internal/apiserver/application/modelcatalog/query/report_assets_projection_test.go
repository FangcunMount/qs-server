package query

import (
	"bytes"
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedPublicDetailDoesNotExposeInternalProjectionVersion(t *testing.T) {
	var baseline []byte
	for _, version := range []interpretationassets.ProjectionVersion{0, 1, 2, 99} {
		model := &port.PublishedModel{Kind: domain.KindScale, Code: "M", Version: "v1", DefinitionV2: &domain.Definition{}, ReportAssetsProjectionVersion: version}
		detail, err := publishedDetailFromModel(model)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(detail)
		if err != nil {
			t.Fatal(err)
		}
		if version == 0 {
			baseline = raw
		}
		if !bytes.Equal(baseline, raw) {
			t.Fatal("internal marker changed public response")
		}
	}
}
