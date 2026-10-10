package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
)

func TestHistoricalSourceInputFrozenRecipeDoesNotRenewOldCapability(t *testing.T) {
	f := sourceAuthFixture(t, false, true)
	copies, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOriginCopies(t.Context(), copies, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := FreezeHistoricalSourceInputRecipe(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	started := binding.started
	binding.started = started.Add(-2 * time.Hour)
	if binding.alive(t.Context()) == nil || !recipe.valid() || recipe.binding.started != started {
		t.Fatal("input freeze renewed or requires old authority")
	}
	if _, err = FreezeHistoricalSourceInputRecipe(t.Context(), binding); err == nil {
		t.Fatal("expired old binding minted new input")
	}
	for _, v := range []any{recipe, &HistoricalSourceInputEpoch{}, &HistoricalSourceInputPair{}} {
		if _, err = json.Marshal(v); err == nil {
			t.Fatal("opaque input serialized")
		}
	}
	altered := *recipe
	if altered.valid() {
		t.Fatal("copied recipe pointer accepted")
	}
	recipe.binding.expected[0].Records++
	if recipe.valid() {
		t.Fatal("modified baseline accepted")
	}
	if _, err = CompareIndependentHistoricalSourceInputs(context.Background(), nil, nil); err == nil {
		t.Fatal("absent full inputs accepted")
	}
}

func TestHistoricalComponentSourceSelectedOriginalFrameAndTamper(t *testing.T) {
	f := wholeJointUnitFixture(t, 2, true)
	c, err := PrepareHistoricalCoordinator(t.Context(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal(err)
	}
	copies := make([]WholeSourceJointCopy, 4)
	for i := range copies {
		file, e := os.OpenFile(filepath.Join(t.TempDir(), "source"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if e := file.Close(); e != nil {
				t.Error(e)
			}
		})
		if _, e = file.Write(f.raw[i]); e != nil {
			t.Fatal(e)
		}
		copies[i] = WholeSourceJointCopy{Input: file, Expected: f.expected[i]}
	}
	index, err := c.PrepareWholeSourceJointIndex(t.Context(), copies, DefaultWholeSourceJointLimits())
	if err != nil {
		t.Fatal(err)
	}
	// This fixture supplies only the private byte matching input. It has no
	// actual SQL/Mongo epoch and cannot satisfy the public read producer.
	pair := &HistoricalSourceInputPair{second: &HistoricalSourceInputEpoch{recipe: &HistoricalSourceInputRecipe{binding: OriginCopyBinding{expected: f.expected, fileHashes: index.encodedSHA}}, receipts: index.receipts}}
	pair.self = pair
	c.started = time.Now().Add(-2 * time.Hour)
	for _, id := range []string{"coordinator-sql-0", "coordinator-mongo-0"} {
		facts, e := sourceComponentFrozenEvent(t.Context(), index, pair, id)
		if e != nil || facts.EventID != id {
			t.Fatal("selected immutable frame rejected", e)
		}
		entry := index.entries[id]
		file := copies[entry.Key.object].Input.(*os.File)
		value := []byte{0}
		if _, e = file.ReadAt(value, entry.Offset); e != nil {
			t.Fatal(e)
		}
		original := value[0]
		value[0] ^= 1
		if _, e = file.WriteAt(value, entry.Offset); e != nil {
			t.Fatal(e)
		}
		if _, e = sourceComponentFrozenEvent(t.Context(), index, pair, id); e == nil {
			t.Fatal("changed same-inode frame accepted")
		}
		value[0] = original
		if _, e = file.WriteAt(value, entry.Offset); e != nil {
			t.Fatal(e)
		}
	}
	if _, err = PrepareHistoricalComponentSourceObservation(t.Context(), nil, pair, nil, nil, time.Second); err == nil {
		t.Fatal("byte-only input supplied real read authority")
	}
}

func TestHistoricalComponentSourceMongoNegativeIndexEligibility(t *testing.T) {
	base := bson.D{{Key: "name", Value: "idx_outbox_consistency_audit"}, {Key: "key", Value: bson.D{{Key: "aggregate_type", Value: 1}, {Key: "event_type", Value: 1}, {Key: "aggregate_id", Value: 1}}}}
	for _, tc := range []struct {
		name     string
		extra    bson.E
		accepted bool
	}{
		{"complete", bson.E{}, true},
		{"partial", bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "status", Value: "published"}}}, false},
		{"sparse", bson.E{Key: "sparse", Value: true}, false},
		{"locale", bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "en"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := append(bson.D(nil), base...)
			if tc.extra.Key != "" {
				doc = append(doc, tc.extra)
			}
			raw, e := bson.Marshal(doc)
			if e != nil {
				t.Fatal(e)
			}
			meta := mongoCycleMetadata{definitions: map[string]mongoCycleDefinition{"domain_event_outbox": {indexes: []bson.Raw{raw}}}}
			if (sourceComponentMongoIndex(meta) == nil) != tc.accepted {
				t.Fatal("negative-range index eligibility mismatch")
			}
		})
	}
	for _, keys := range []any{nil, "bad", bson.D{{Key: "aggregate_type", Value: 1}, {Key: "aggregate_id", Value: 1}, {Key: "event_type", Value: 1}}} {
		raw, e := bson.Marshal(bson.D{{Key: "name", Value: "idx_outbox_consistency_audit"}, {Key: "key", Value: keys}})
		if e != nil {
			t.Fatal(e)
		}
		meta := mongoCycleMetadata{definitions: map[string]mongoCycleDefinition{"domain_event_outbox": {indexes: []bson.Raw{raw}}}}
		if sourceComponentMongoIndex(meta) == nil {
			t.Fatal("wrong or malformed index accepted")
		}
	}
}

func TestHistoricalComponentSourceNeverClaimsOwnerClosureOrCAS(t *testing.T) {
	var absent *HistoricalComponentSourceObservation
	r := absent.Summary()
	if !r.SQLSourceNegativeClosureRequired || !r.MongoOwnerSourceNegativeClosureRequired || !r.SourceWriterFenceRequired || !r.CurrentMessageClosureRequired || !r.AIClosureRequired || r.SourceClosureVerified || r.CASAuthorized || r.DropReady {
		t.Fatal("read-only source facts overstated closure")
	}
	if absent.ValidateBorrowedObservation(t.Context()) == nil {
		t.Fatal("missing actual native scope accepted")
	}
	if _, err := json.Marshal(&HistoricalComponentSourceObservation{}); err == nil {
		t.Fatal("private source observation serialized")
	}
	var component *HistoricalComponentObservation
	combined := component.Summary()
	if !combined.FullNegativeClosureRequired || !combined.WriterFenceRequired || !combined.Source.SQLSourceNegativeClosureRequired || !combined.Source.AIClosureRequired || !combined.AI.UnboundOrphanNegativeClosureRequired || !combined.AI.NewOwnerOrganizationNegativeClosureRequired || combined.SameNativeScopesObserved || combined.CASAuthorized || combined.DropReady {
		t.Fatal("missing combined scopes erased outstanding qualification")
	}
	if component.ValidateBorrowedObservation(t.Context()) == nil {
		t.Fatal("absent combined native scopes accepted")
	}
	if _, err := json.Marshal(&HistoricalComponentObservation{}); err == nil {
		t.Fatal("combined private candidates serialized")
	}
}

func TestHistoricalSourceInputIndexActualCopiesAndLegacySeparation(t *testing.T) {
	f := wholeJointUnitFixture(t, 2, true)
	auth, err := VerifySourceCopies(t.Context(), f.inputs())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOriginCopies(t.Context(), auth, f.inputs(), DefaultSourceOriginLimits())
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := FreezeHistoricalSourceInputRecipe(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	// Frozen input can survive the old authority; indexing must neither renew
	// nor reconstruct that old authority to read the exact approved bytes.
	binding.started = binding.started.Add(-2 * time.Hour)
	x, err := PrepareHistoricalSourceInputIndex(t.Context(), coordinatorBinding(), recipe, wholeJointCopies(f), DefaultWholeSourceJointLimits())
	if err != nil || x == nil || x.self != x || x.owner != nil || x.auth != auth || x.input != recipe || x.binding != coordinatorBinding() || len(x.entries) != 6 || x.encodedSHA != recipe.binding.fileHashes || x.receipts != auth.receipts || x.Summary().DropReady || binding.alive(t.Context()) == nil {
		t.Fatal("actual pure input index rejected or old authority reconstructed", err)
	}
	for _, id := range []string{"coordinator-sql-0", "coordinator-mongo-0"} {
		facts, err := x.readEvent(t.Context(), id)
		if err != nil || facts.EventID != id {
			t.Fatal("original frame rejected", err)
		}
		if _, err = x.event(t.Context(), id); !errors.Is(err, ErrWholeSourceJoint) {
			t.Fatal("pure index entered legacy bound event path", err)
		}
	}
	if x.RecheckSourceCopies(t.Context(), wholeJointCopies(f)) == nil {
		t.Fatal("pure index entered legacy coordinator recheck")
	}
	if _, err = x.InputEvents(t.Context(), nil, []string{"coordinator-sql-0"}); err == nil {
		t.Fatal("bytes alone issued handles without two native scopes")
	}
	if x.ReleaseInputAuthentication(t.Context(), nil) == nil || x.auth != auth {
		t.Fatal("missing native pair released authenticated planning membership")
	}
	clone := *recipe
	if _, err = PrepareHistoricalSourceInputIndex(t.Context(), coordinatorBinding(), &clone, wholeJointCopies(f), DefaultWholeSourceJointLimits()); err == nil {
		t.Fatal("copied recipe accepted")
	}
	changed := wholeJointCopies(f)
	// Equivalent decoded JSON facts do not excuse a different encoded file.
	changed[0].Input = bytes.NewReader(bytes.Replace(f.raw[0], []byte(`"protocol":`), []byte(`"protocol" :`), 1))
	if y, err := PrepareHistoricalSourceInputIndex(t.Context(), coordinatorBinding(), recipe, changed, DefaultWholeSourceJointLimits()); !errors.Is(err, ErrSourceAuthentication) || y != nil {
		t.Fatal("different source bytes with identical facts accepted", err)
	}
	recipe.captureStopped = true
	if _, err = PrepareHistoricalSourceInputIndex(t.Context(), coordinatorBinding(), recipe, wholeJointCopies(f), DefaultWholeSourceJointLimits()); err == nil {
		t.Fatal("released recipe rebuilt membership")
	}
}

func TestHistoricalComponentBusinessRowsRequireActualSourceObservation(t *testing.T) {
	for _, observation := range []*HistoricalComponentSourceObservation{nil, {}, {rowsSHA: "editable-summary"}} {
		if rows, err := qualifiedHistoricalComponentBusinessRows(t.Context(), observation); rows != nil || err == nil {
			t.Fatal("editable or absent input became business qualification")
		}
	}
	reader := &historicalComponentSQLOwnerReader{ctx: t.Context()}
	if reader.Snapshot().Owner.AssessmentID != 0 || reader.HasVerifiedAnswerSheetAssociation(42) {
		t.Fatal("absent native owner returned editable business facts")
	}
	if _, err := reader.OutcomeRecord(42); err == nil {
		t.Fatal("absent native outcome accepted")
	}
	// Exercise the private entry format after the producer's gap classification;
	// these byte-format fixtures cannot create an actual source observation.
	row := qualifiedCASEntryFixture("answersheet.submitted")
	row.sourceObservation = &HistoricalComponentSourceObservation{}
	gaps := []string{"sql_retry_event_hold_and_dead_letter_not_mongo_stores", "inbox_and_global_unbound_coverage_require_actual_runtime_coordinator", "storage_precision_gap"}
	before := append([]string(nil), gaps...)
	appendHistoricalComponentMongoGaps(&row.candidate, gaps)
	entry, err := qualifiedCASReferenceEntry(coordinatorBinding(), row, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC), "actual-fresh-owner-component-source-business-related-ai")
	if err != nil || entry.Proof == nil || entry.Proof.Class != evidence.Unverifiable || entry.Proof.Verification.Reason != "storage_precision_gap" {
		t.Fatal("current coverage hints became historical evidence gaps", err)
	}
	if !slices.Equal(row.candidate.HistoricalGaps, []string{"storage_precision_gap"}) || !slices.Equal(before, gaps) || !slices.Contains(row.candidate.RequiredAdapters, gaps[0]) || !slices.Contains(row.candidate.RequiredAdapters, gaps[1]) {
		t.Fatal("coverage requirements or original gap data lost")
	}
	appendHistoricalComponentMongoGaps(&row.candidate, []string{"unknown_current_responsibility"})
	if _, err = qualifiedCASReferenceEntry(coordinatorBinding(), row, time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC), "actual-fresh-owner-component-source-business-related-ai"); !errors.Is(err, ErrCoordinatorCASQualification) {
		t.Fatal("unknown resolver gap became historical evidence")
	}
}

func TestHistoricalComponentSourceSQLIndexEligibility(t *testing.T) {
	valid := func() SQLColumns {
		var rows SQLColumns
		for i, name := range []string{"aggregate_type", "aggregate_id", "event_type", "id"} {
			values := []string{"idx_outbox_aggregate_event_latest", strconv.Itoa(i + 1), name, "", "A", "1", "BTREE", "YES"}
			row := make([]*string, len(values))
			for j := range values {
				if j != 3 {
					v := values[j]
					row[j] = &v
				}
			}
			rows = append(rows, row)
		}
		return rows
	}
	if err := sourceComponentSQLIndex(valid()); err != nil {
		t.Fatal("actual full index definition rejected", err)
	}
	for _, tc := range []struct {
		name   string
		column int
		value  string
	}{
		{"other-index", 0, "other"}, {"wrong-sequence", 1, "2"},
		{"wrong-column", 2, "event_type"}, {"prefix", 3, "16"},
		{"descending", 4, "D"}, {"unique", 5, "0"},
		{"hash", 6, "HASH"}, {"invisible", 7, "NO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := valid()
			rows[0][tc.column] = &tc.value
			if sourceComponentSQLIndex(rows) == nil {
				t.Fatal("ineligible negative-range index accepted")
			}
		})
	}
	for _, rows := range []SQLColumns{nil, valid()[:3], append(valid(), valid()[0])} {
		if sourceComponentSQLIndex(rows) == nil {
			t.Fatal("missing or extra physical index columns accepted")
		}
	}
}

func TestHistoricalComponentSourceSQLRangesIncludeOwnersWithoutSources(t *testing.T) {
	c := &HistoricalCASComponent{inputs: []*HistoricalCASComponentInput{{owners: []historicalCASOwnerKey{{"assessment", 42, 7}, {"assessment", 43, 8}, {"sheet", 10042, 7}}}}}
	expected := map[string]*DecodedSourceEvent{
		"sql":   {AggregateType: "Evaluation", AggregateID: "42"},
		"mongo": {AggregateType: "Questionnaire", AggregateID: "10042"},
	}
	groups, err := sourceComponentSQLRanges(c, expected)
	if err != nil || len(groups) != 2 || len(groups["Evaluation"]) != 2 || !groups["Evaluation"]["42"] || !groups["Evaluation"]["43"] || !groups["Questionnaire"]["10042"] {
		t.Fatal("SQL-empty/cross-org actual owner or Mongo aggregate omitted", err)
	}
	c.inputs[0].owners[1].org = 0
	if _, err = sourceComponentSQLRanges(c, expected); err == nil {
		t.Fatal("unproven owner organization accepted")
	}
	if _, err = sourceComponentSQLRanges(&HistoricalCASComponent{}, map[string]*DecodedSourceEvent{"nil": nil}); err == nil {
		t.Fatal("malformed source aggregate accepted")
	}
}
