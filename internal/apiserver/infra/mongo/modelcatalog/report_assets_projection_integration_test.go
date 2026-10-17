//go:build integration

package modelcatalog

import (
	"bytes"
	"errors"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestReportAssetsVersionRealMongoLegacyIdempotenceAndNewWrites(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repo := NewRepository(db)
	model := &port.PublishedModel{Kind: domain.KindTypology, Code: "LEGACY", Version: "v1", DefinitionV2: durableProjectionDefinition()}
	legacy := NewMapper().ToPO(model)
	legacy.BeforeInsert()
	if _, err := db.Collection(legacy.CollectionName()).InsertOne(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	readRaw := func(code string) bson.Raw {
		t.Helper()
		raw, err := db.Collection(legacy.CollectionName()).FindOne(t.Context(), bson.M{"code": code, "record_role": recordRolePublishedSnapshot}).DecodeBytes()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := readRaw(model.Code)
	loaded, err := repo.GetPublishedModelByRef(t.Context(), port.Ref{Kind: model.Kind, Code: model.Code, Version: model.Version})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReportAssetsProjectionVersion != 0 {
		t.Fatal("read silently assigned legacy version")
	}
	loaded.ReportAssetsProjectionVersion = 1
	if err := repo.Save(t.Context(), loaded); err != nil {
		t.Fatalf("legacy idempotent save: %v", err)
	}
	if !bytes.Equal(before, readRaw(model.Code)) {
		t.Fatal("idempotent save rewrote legacy BSON")
	}

	loaded.ReportAssetsProjectionVersion = 2
	if err := repo.Save(t.Context(), loaded); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("V2 write opened: %v", err)
	}
	if !bytes.Equal(before, readRaw(model.Code)) {
		t.Fatal("rejected V2 write mutated legacy BSON")
	}
	model.Code, model.Version = "NEW", "v2"
	if err := repo.Save(t.Context(), model); err != nil {
		t.Fatal(err)
	}
	if got := readRaw(model.Code).Lookup("report_assets_projection_version").Int32(); got != 1 {
		t.Fatalf("new marker = %d", got)
	}

	// Read compatibility remains open for an unknown stored marker; no writes.
	if _, err := db.Collection(legacy.CollectionName()).UpdateOne(t.Context(), bson.M{"code": "LEGACY"}, bson.M{"$set": bson.M{"report_assets_projection_version": 99}}); err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.GetPublishedModelByRef(t.Context(), port.Ref{Kind: domain.KindTypology, Code: "LEGACY", Version: "v1"})
	if err != nil || loaded.ReportAssetsProjectionVersion != 99 {
		t.Fatalf("compatibility read = %#v, %v", loaded, err)
	}
}

func TestReportAssetsVersionRealMongoHeadEditDoesNotBackfillLegacyReads(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repo := NewDraftRepository(db)
	model := &domain.AssessmentModel{Code: "HEAD", Kind: domain.KindScale, Title: "old", Status: domain.ModelStatusDraft, Version: 1, DefinitionV2: sampleDefinitionV2()}
	legacy := NewDraftMapper().ToPO(model)
	legacy.BeforeInsert()
	if _, err := db.Collection(legacy.CollectionName()).InsertOne(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.FindByCode(t.Context(), model.Code)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReportAssetsProjectionVersion != 0 {
		t.Fatal("head read assigned marker")
	}
	if err := loaded.UpdateBasicInfo("metadata edit", "", loaded.Algorithm, "", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(t.Context(), loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.FindByCode(t.Context(), model.Code)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReportAssetsProjectionVersion != 0 {
		t.Fatal("metadata edit backfilled asset version")
	}
	if err := loaded.UpdateDefinition(sampleDefinitionV2(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(t.Context(), loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.FindByCode(t.Context(), model.Code)
	if err != nil || loaded.ReportAssetsProjectionVersion != interpretationassets.ProjectionLegacyV1 {
		t.Fatalf("Definition edit = %#v, %v", loaded, err)
	}
}
