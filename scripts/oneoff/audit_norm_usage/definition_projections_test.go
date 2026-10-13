package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func projectionTestConfig() config {
	return config{definitionProjections: true, mongoURI: "mongodb://127.0.0.1:1", mongoDB: "qs_m5_audit_test", batchSize: 1, maxRecords: 100, timeout: time.Minute}
}

func projectionTestDoc(t *testing.T, family string) bson.M {
	t.Helper()
	def := &modeldefinition.Definition{
		Measure:     modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Outcomes:    []conclusion.Outcome{{Code: "low", Summary: "NEVER_PRINT_REPORT_TEXT"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}}}},
	}
	algorithm := domain.AlgorithmScaleDefault
	switch family {
	case "behavioral_rating":
		algorithm = domain.AlgorithmBrief2
		def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: "test-v1"}}
		def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true, Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}}}}
	case "cognitive":
		algorithm = domain.AlgorithmSPM
		def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true, Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}}}}
	case "typology":
		algorithm = domain.AlgorithmPersonalityTypology
		def.Conclusions = []conclusion.Conclusion{conclusion.TypeConclusion{FactorCodes: []string{"total"}, Decision: conclusion.TypeDecision{Kind: binding.DecisionKindTraitProfile}, Profiles: []conclusion.TypeOutcomeProfile{{OutcomeCode: "low"}}}}
	}
	modeldefinition.MaterializeLayers(def)
	decision, err := (&domain.AssessmentModel{Kind: domain.Kind(family), DefinitionV2: def}).DecisionKindForDefinition()
	if err != nil {
		t.Fatal(err)
	}
	model := &modelport.PublishedModel{Code: "test-model", ReleaseStatus: domain.ReleaseStatusActive, Kind: domain.Kind(family), Algorithm: algorithm, DecisionKind: decision, Version: "v1", DefinitionV2: def}
	hash, err := modeldefinition.CanonicalContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	modelport.AttachDefinitionHash(model, hash)
	po := mongomodelcatalog.NewMapper().ToPO(model)
	// Valid stored-metadata fixtures bind the exact decoded canonical shape.
	// A separate test preserves the real pre-storage hash mismatch case.
	storedHash, err := modeldefinition.CanonicalContentHash(mongomodelcatalog.NewMapper().ToPublished(po).DefinitionV2)
	if err != nil {
		t.Fatal(err)
	}
	po.Source[modelport.SourceDefinitionContentHash] = storedHash
	po.ID = primitive.NewObjectID()
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestProjectionRecordVerifiesPublishedHashMetadataWithoutDisclosure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(bson.M)
		reason string
	}{
		{"missing source", func(d bson.M) { delete(d, "source") }, "metadata.definition_hash_source_invalid"},
		{"null source", func(d bson.M) { d["source"] = nil }, "metadata.definition_hash_source_invalid"},
		{"non-document source", func(d bson.M) { d["source"] = "NEVER_PRINT_SOURCE" }, "metadata.definition_hash_source_invalid"},
		{"missing schema", func(d bson.M) { delete(d["source"].(bson.M), modelport.SourceDefinitionHashSchema) }, "metadata.definition_hash_schema_invalid"},
		{"unknown schema", func(d bson.M) { d["source"].(bson.M)[modelport.SourceDefinitionHashSchema] = "NEVER_PRINT_SCHEMA" }, "metadata.definition_hash_schema_invalid"},
		{"typed schema", func(d bson.M) { d["source"].(bson.M)[modelport.SourceDefinitionHashSchema] = 1 }, "metadata.definition_hash_schema_invalid"},
		{"missing hash", func(d bson.M) { delete(d["source"].(bson.M), modelport.SourceDefinitionContentHash) }, "metadata.definition_content_hash_invalid"},
		{"malformed hash", func(d bson.M) { d["source"].(bson.M)[modelport.SourceDefinitionContentHash] = "NEVER_PRINT_HASH" }, "metadata.definition_content_hash_invalid"},
		{"typed hash", func(d bson.M) { d["source"].(bson.M)[modelport.SourceDefinitionContentHash] = 1 }, "metadata.definition_content_hash_invalid"},
		{"uppercase hash", func(d bson.M) {
			s := d["source"].(bson.M)
			s[modelport.SourceDefinitionContentHash] = strings.ToUpper(s[modelport.SourceDefinitionContentHash].(string))
		}, "metadata.definition_content_hash_invalid"},
		{"mismatched hash", func(d bson.M) { d["source"].(bson.M)[modelport.SourceDefinitionContentHash] = strings.Repeat("0", 64) }, "metadata.definition_content_hash_mismatch"},
		{"duplicate source keys", func(d bson.M) {
			d["source"] = bson.D{{Key: modelport.SourceDefinitionHashSchema, Value: "definition-v2/v1"}, {Key: modelport.SourceDefinitionHashSchema, Value: "NEVER_PRINT_DUPLICATE"}}
		}, "raw.duplicate_key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := projectionTestDoc(t, "scale")
			tc.mutate(doc)
			record := recordFromDoc(t, doc)
			if record.Status != "matched" || record.HashMetadataStatus != "invalid" || record.HashMetadataReasonCounts[tc.reason] != 1 || record.CanonicalHash == "" || record.Revision != "v1" || len(record.Fingerprint) != 64 {
				t.Fatalf("published hash metadata was not blocked: %+v", record)
			}
			encoded, err := json.Marshal(record)
			if err != nil || strings.Contains(string(encoded), "NEVER_PRINT") {
				t.Fatal("source value disclosed")
			}
		})
	}
	for _, family := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		doc := projectionTestDoc(t, family)
		if got := recordFromDoc(t, doc); got.Status != "matched" || got.HashMetadataStatus != "matched" {
			t.Fatalf("valid %s published hash rejected: %+v", family, got)
		}
		doc["record_role"], doc["revision"] = "head", int64(9)
		delete(doc, "source")
		if got := recordFromDoc(t, doc); got.Status != "matched" || got.HashMetadataStatus != "not_applicable" {
			t.Fatalf("snapshot-only source required on %s head: %+v", family, got)
		}
	}
}

func TestProjectionRecordRetainsPreStorageHashMismatchAsIndependentFinding(t *testing.T) {
	def := &modeldefinition.Definition{
		Measure:     modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Outcomes:    []conclusion.Outcome{{Code: "low"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}}}},
	}
	modeldefinition.MaterializeLayers(def)
	before, err := modeldefinition.CanonicalContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	model := &modelport.PublishedModel{Code: "roundtrip-model", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, DecisionKind: domain.DecisionKindScoreRange, Version: "v1", DefinitionV2: def}
	modelport.AttachDefinitionHash(model, before)
	po := mongomodelcatalog.NewMapper().ToPO(model)
	po.ID = primitive.NewObjectID()
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	record := auditProjectionRecord(raw)
	if record.CanonicalHash == before || record.Status != "matched" || record.HashMetadataStatus != "invalid" || record.HashMetadataReasonCounts["metadata.definition_content_hash_mismatch"] != 1 {
		t.Fatalf("pre-storage hash mismatch was hidden or relabeled as an asset mismatch: %+v", record)
	}
	// Existing storage normalization is observed, never repaired by this audit.
	if def.Calibration.NormRefs != nil || model.Source[modelport.SourceDefinitionContentHash] != before {
		t.Fatal("audit rehashed or changed its input")
	}
}

func recordFromDoc(t *testing.T, doc bson.M) projectionRecord {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return auditProjectionRecord(raw)
}

func TestProjectionRecordBindsIdentityRevisionFingerprintAndBlocksMetadata(t *testing.T) {
	doc := projectionTestDoc(t, "scale")
	first := recordFromDoc(t, doc)
	if first.Status != "matched" || first.Revision != "v1" || len(first.Fingerprint) != 64 || len(first.CanonicalHash) != 64 || first.ModelID == "" {
		t.Fatalf("record=%+v", first)
	}
	doc["record_role"], doc["revision"] = "head", int64(9)
	head := recordFromDoc(t, doc)
	if head.Status != "matched" || head.Revision != "9" || head.Fingerprint == first.Fingerprint {
		t.Fatalf("head=%+v", head)
	}
	var duplicate bson.D
	raw, _ := bson.Marshal(doc)
	if err := bson.Unmarshal(raw, &duplicate); err != nil {
		t.Fatal(err)
	}
	duplicate = append(duplicate, bson.E{Key: "revision", Value: int64(10)})
	raw, _ = bson.Marshal(duplicate)
	if record := auditProjectionRecord(raw); record.Status != "invalid" || record.InvalidReasonCounts["raw.duplicate_key"] != 1 {
		t.Fatal("ambiguous metadata was classified as matching")
	}
	for _, tc := range []struct {
		field  string
		value  any
		status string
	}{
		{"definition_v2", nil, "missing_definition"}, {"algorithm", "future-algorithm", "invalid"}, {"definition_schema_version", "future", "unknown_schema"}, {"revision", int64(0), "invalid"}, {"kind", "future", "unknown_schema"}, {"definition_v2", "private invalid body", "invalid"},
	} {
		copy := bson.M{}
		for k, v := range doc {
			copy[k] = v
		}
		copy[tc.field] = tc.value
		record := recordFromDoc(t, copy)
		if record.Status != tc.status {
			t.Fatalf("%s status=%s want%s", tc.field, record.Status, tc.status)
		}
		encoded, _ := json.Marshal(record)
		if strings.Contains(string(encoded), "NEVER_PRINT_REPORT_TEXT") || strings.Contains(string(encoded), "private invalid body") {
			t.Fatal("report text disclosed")
		}
	}
}

func TestProjectionRecordReportsBoundedReasonsWithSameIdentityAndInvalidStatus(t *testing.T) {
	doc := projectionTestDoc(t, "scale")
	doc["decision_kind"] = "NEVER_PRINT_PRIVATE_DECISION_KIND"
	record := recordFromDoc(t, doc)
	if record.Status != "invalid" || record.InvalidCount != 1 || record.InvalidReasonCounts["metadata.decision_kind_invalid"] != 1 || record.CanonicalHash == "" || record.Revision != "v1" || len(record.Fingerprint) != 64 {
		t.Fatalf("metadata diagnostic changed result or identity: %+v", record)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "NEVER_PRINT") {
		t.Fatal("metadata value disclosed through reason")
	}
}

func TestProjectionConfigBoundsPrivateFileAndCredentials(t *testing.T) {
	for _, mutate := range []func(*config){
		func(c *config) { c.batchSize = 0 }, func(c *config) { c.batchSize = 1001 }, func(c *config) { c.maxRecords = 0 }, func(c *config) { c.maxRecords = 100001 }, func(c *config) { c.timeout = 0 }, func(c *config) { c.timeout = 6 * time.Minute }, func(c *config) { c.normVersion = "partial-filter" },
		func(c *config) { c.uriFromFlag = true; c.mongoURI = "mongodb://user:FAKE_SECRET@localhost" }, func(c *config) {
			c.uriFromFlag = true
			c.mongoURI = "mongodb://localhost/?tlsCertificateKeyFilePassword=FAKE_SECRET"
		},
	} {
		cfg := projectionTestConfig()
		mutate(&cfg)
		var output bytes.Buffer
		if code := runDefinitionProjectionAudit(cfg, &output); code != 1 || strings.Contains(output.String(), "FAKE_SECRET") {
			t.Fatalf("config error code=%d output=%s", code, output.String())
		}
	}
	file := filepath.Join(t.TempDir(), "private.json")
	if err := os.WriteFile(file, []byte(`{"mongo_uri":"mongodb://user:FAKE_SECRET@localhost","mongo_db":"qs_m5_private_test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := projectionTestConfig()
	cfg.privateConfig = file
	resolved, err := projectionConfig(cfg)
	if err != nil || resolved.mongoDB != "qs_m5_private_test" {
		t.Fatal("private configuration was not loaded")
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := projectionConfig(cfg); err == nil {
		t.Fatal("publicly readable connection file accepted")
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	link := file + ".link"
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	cfg.privateConfig = link
	if _, err := projectionConfig(cfg); err == nil {
		t.Fatal("symlink connection file accepted")
	}
}

func TestProjectionFlagsHelpDoesNotRevealEnvironmentURI(t *testing.T) {
	previousFlags, previousArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = previousFlags, previousArgs })
	flag.CommandLine = flag.NewFlagSet("audit", flag.ContinueOnError)
	var help bytes.Buffer
	flag.CommandLine.SetOutput(&help)
	os.Args = []string{"audit", "--help"}
	t.Setenv("MONGO_URI", "mongodb://user:FAKE_SECRET@localhost")
	_ = parseFlags()
	if strings.Contains(help.String(), "FAKE_SECRET") || !strings.Contains(help.String(), "definition-projections") {
		t.Fatal("help disclosed environment URI or omitted audit mode")
	}
}

func TestProjectionEmptyIncompleteAndFailedScansAreNotZero(t *testing.T) {
	for _, result := range []projectionReport{
		{Complete: true, Reason: "empty_collection"}, {Expected: 2, Scanned: 1, Truncated: true}, {ScanErrors: 1}, {Complete: true, Scanned: 1, RecordErrors: 1}, {Complete: true, Scanned: 1, MissingDefinition: 1}, {Complete: true, Scanned: 1, UnknownSchema: 1}, {Complete: true, Scanned: 1, Invalid: 1}, {Complete: true, Scanned: 1, Mismatched: 1},
	} {
		if result.exitCode() == 0 || result.ZeroMismatch {
			t.Fatal("nonzero/incomplete result accepted")
		}
	}
}
