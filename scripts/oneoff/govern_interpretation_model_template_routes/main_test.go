package main

import (
	"crypto/sha256"
	"strings"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestAttachCanonicalHashesUsesDomainDefinitionHash(t *testing.T) {
	definition := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total"}}},
		ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{
			Code: "result", Kind: modeldefinition.ReportSectionKindFactorScores, TemplateID: "standard",
		}}},
	}
	model := &modelcatalogport.PublishedModel{
		Kind: domain.KindScale, Code: "MODEL", Version: "v3", Source: map[string]any{}, DefinitionV2: definition,
	}
	po := mongomodelcatalog.NewMapper().ToPO(model)
	roundTrip := mongomodelcatalog.NewMapper().ToPublished(po)
	sourceHash, err := modeldefinition.CanonicalContentHash(roundTrip.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	po.Source[modelcatalogport.SourceDefinitionContentHash] = sourceHash
	po.Source[modelcatalogport.SourceDefinitionHashSchema] = "definition-v2/v1"
	record := map[string]any{
		"kind": "scale", "code": "MODEL", "source_release_version": "v3", "template_id": "standard",
		"factor_score_section_added": false, "factor_source_refs": []any{},
	}
	if err := attachCanonicalHashes(record, po); err != nil {
		t.Fatal(err)
	}
	if record["source_definition_hash"] != sourceHash {
		t.Fatalf("source hash = %v, want %s", record["source_definition_hash"], sourceHash)
	}
	targetHash, _ := record["target_definition_hash"].(string)
	if !isSHA256(targetHash) || targetHash == sourceHash {
		t.Fatalf("target hash = %q, source = %q", targetHash, sourceHash)
	}

	target, err := cloneDefinition(roundTrip.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	target.ReportMap.Sections[0].TemplateVersion = targetTemplateVersion
	modeldefinition.MaterializeLayers(target)
	wantTargetHash, err := modeldefinition.CanonicalContentHash(target)
	if err != nil {
		t.Fatal(err)
	}
	if targetHash != wantTargetHash {
		t.Fatalf("target hash = %s, want %s", targetHash, wantTargetHash)
	}
}

func TestAttachCanonicalHashesRejectsStaleStoredHash(t *testing.T) {
	definition := &modeldefinition.Definition{ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{Code: "result"}}}}
	model := &modelcatalogport.PublishedModel{
		Code: "MODEL", Version: "v3", Source: map[string]any{modelcatalogport.SourceDefinitionContentHash: strings.Repeat("a", sha256.Size*2)}, DefinitionV2: definition,
	}
	po := mongomodelcatalog.NewMapper().ToPO(model)
	err := attachCanonicalHashes(map[string]any{
		"code": "MODEL", "source_release_version": "v3", "template_id": "standard",
		"factor_score_section_added": false, "factor_source_refs": []any{},
	}, po)
	if err == nil || !strings.Contains(err.Error(), "stored source definition hash") {
		t.Fatalf("error = %v", err)
	}
}

func TestAttachCanonicalHashesIncludesMaterializedFactorVisibility(t *testing.T) {
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{
		{Code: "total"}, {Code: "detail"},
	}}}
	model := &modelcatalogport.PublishedModel{
		Kind: domain.KindScale, Code: "MODEL", Version: "v3", Source: map[string]any{}, DefinitionV2: definition,
	}
	po := mongomodelcatalog.NewMapper().ToPO(model)
	roundTrip := mongomodelcatalog.NewMapper().ToPublished(po)
	sourceHash, err := modeldefinition.CanonicalContentHash(roundTrip.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	po.Source[modelcatalogport.SourceDefinitionContentHash] = sourceHash
	po.Source[modelcatalogport.SourceDefinitionHashSchema] = "definition-v2/v1"
	record := map[string]any{
		"kind": "scale", "code": "MODEL", "source_release_version": "v3", "template_id": "standard",
		"factor_score_section_added": true, "factor_source_refs": []any{"total", "detail"},
	}
	if err := attachCanonicalHashes(record, po); err != nil {
		t.Fatal(err)
	}

	target, err := cloneDefinition(roundTrip.DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := materializeFactorScoreSection(target, "scale"); err != nil {
		t.Fatal(err)
	}
	for index := range target.ReportMap.Sections {
		target.ReportMap.Sections[index].TemplateID = "standard"
		target.ReportMap.Sections[index].TemplateVersion = targetTemplateVersion
	}
	modeldefinition.MaterializeLayers(target)
	wantTargetHash, err := modeldefinition.CanonicalContentHash(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := record["target_definition_hash"]; got != wantTargetHash {
		t.Fatalf("target hash = %v, want %s", got, wantTargetHash)
	}
}

func TestManifestValidationRequiresCanonicalHashesAfterEnrichment(t *testing.T) {
	records := []map[string]any{{
		"source_id":                  "000000000000000000000001",
		"factor_score_section_added": true,
		"factor_source_refs":         []any{"total"},
	}}
	fingerprint, err := recordsFingerprint(records)
	if err != nil {
		t.Fatal(err)
	}
	manifest := governanceManifest{
		SchemaVersion: governanceSchemaVersion, TargetTemplateVersion: targetTemplateVersion,
		Records: records, RecordsFingerprint: fingerprint,
	}
	if err := validateManifest(manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := validateManifest(manifest, true); err == nil {
		t.Fatal("expected missing canonical hashes to be rejected")
	}
	records[0]["source_definition_hash"] = strings.Repeat("a", sha256.Size*2)
	records[0]["target_definition_hash"] = strings.Repeat("b", sha256.Size*2)
	manifest.RecordsFingerprint, _ = recordsFingerprint(records)
	if err := validateManifest(manifest, true); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeFactorScoreSectionDerivesAllMeasureFactors(t *testing.T) {
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{Factors: []factor.Factor{
		{Code: "total"}, {Code: "detail"},
	}}}
	added, refs, err := materializeFactorScoreSection(definition, "scale")
	if err != nil {
		t.Fatal(err)
	}
	if !added || !equalStrings(refs, []string{"total", "detail"}) {
		t.Fatalf("added = %t, refs = %#v", added, refs)
	}
	want := modeldefinition.ReportSection{
		Code: "factor_scores", Kind: modeldefinition.ReportSectionKindFactorScores,
		SourceRefs: []string{"total", "detail"},
	}
	if len(definition.ReportMap.Sections) != 1 || definition.ReportMap.Sections[0].Code != want.Code ||
		definition.ReportMap.Sections[0].Kind != want.Kind || !equalStrings(definition.ReportMap.Sections[0].SourceRefs, want.SourceRefs) {
		t.Fatalf("sections = %#v", definition.ReportMap.Sections)
	}
}

func TestMaterializeFactorScoreSectionKeepsTypologyStrict(t *testing.T) {
	definition := &modeldefinition.Definition{}
	if _, _, err := materializeFactorScoreSection(definition, "typology"); err == nil || !strings.Contains(err.Error(), "typology") {
		t.Fatalf("error = %v", err)
	}
}

func TestRecordsFingerprintMatchesMongoshCanonicalJSON(t *testing.T) {
	records := []map[string]any{{
		"source_id": "000000000000000000000001", "code": "MODEL", "max_records": float64(0),
		"source_content_hash":    strings.Repeat("a", sha256.Size*2),
		"source_definition_hash": strings.Repeat("b", sha256.Size*2),
		"target_definition_hash": strings.Repeat("c", sha256.Size*2),
	}}
	got, err := recordsFingerprint(records)
	if err != nil {
		t.Fatal(err)
	}
	const want = "9b82af2cb720474633449653a2bb746922ae3d4d62a594bff1dbda6921e33011"
	if got != want {
		t.Fatalf("fingerprint = %s, want mongosh canonical fingerprint %s", got, want)
	}
}
