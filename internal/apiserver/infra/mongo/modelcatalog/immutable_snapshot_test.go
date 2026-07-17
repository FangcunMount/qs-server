package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestSameImmutablePublishedContentIgnoresReleaseStateButRejectsPayloadChange(t *testing.T) {
	active := &port.PublishedModel{Kind: domain.KindScale, Code: "S-1", Version: "2", Payload: []byte(`{"score":1}`), ReleaseStatus: domain.ReleaseStatusActive}
	archived := *active
	archived.ReleaseStatus = domain.ReleaseStatusArchived
	if !sameImmutablePublishedContent(active, &archived) {
		t.Fatal("release metadata must not alter immutable content identity")
	}
	conflict := archived
	conflict.Payload = []byte(`{"score":2}`)
	if sameImmutablePublishedContent(active, &conflict) {
		t.Fatal("payload change under the same release version must conflict")
	}
}
