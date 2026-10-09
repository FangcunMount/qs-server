package retirement

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func persistenceTestReference() (evidence.HistoricalReferenceEntryV1, evidence.Digest, HistoricalCandidate, HistoricalCoordinatorBinding) {
	digest := evidence.SourceDigest("mysql-cast-binary-row-v2", []byte("synthetic original source bytes"))
	content := evidence.SourceDigest("legacy-domain-json-bytes-v1", []byte("synthetic content"))
	entry := evidence.HistoricalReferenceEntryV1{EventID: "original-event", EventType: "evaluation.requested", Source: evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "mysql_uint64", PrimaryKeySHA256: strings.Repeat("1", 64), Digest: digest}, Proof: &evidence.EventEvidenceV1{Version: 1, Class: evidence.RetiredVerified, EventID: "original-event", Digest: digest, BusinessBindingSHA256: strings.Repeat("2", 64), Origin: "retirement", Verification: evidence.Verification{Method: "unit-source-binding-only", Version: "v1", OperationID: "123-1", VerifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}}
	candidate := HistoricalCandidate{Source: entry.Source, OriginalID: entry.EventID, EventType: entry.EventType, OwnerDatabase: "mysql", OwnerObject: "assessment", OwnerID: "42", ContentDigest: content, BusinessBindingSHA256: entry.Proof.BusinessBindingSHA256, LocalQualified: true}
	return entry, content, candidate, HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1"}
}

func TestHistoricalCASPersistenceReferenceDoesNotReplaceOriginalIdentity(t *testing.T) {
	entry, content, candidate, binding := persistenceTestReference()
	if err := casEntryMatchesCandidate(entry, content, candidate, 42, binding); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*evidence.HistoricalReferenceEntryV1, *evidence.Digest, *HistoricalCandidate, *HistoricalCoordinatorBinding){
		"event-id": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.EventID = "other"
		},
		"type": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.EventType = "evaluation.failed"
		},
		"source-pk": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.Source.PrimaryKeySHA256 = strings.Repeat("3", 64)
		},
		"source-bytes": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.Source.Digest.SHA256 = strings.Repeat("3", 64)
		},
		"content": func(_ *evidence.HistoricalReferenceEntryV1, d *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			d.SHA256 = strings.Repeat("3", 64)
		},
		"owner": func(_ *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, c *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			c.OwnerID = "43"
		},
		"operation": func(_ *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, b *HistoricalCoordinatorBinding) {
			b.OperationID = "124-1"
		},
		"stable-binding": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.Proof.BusinessBindingSHA256 = strings.Repeat("3", 64)
		},
		"latest-run-invented": func(e *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, _ *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			e.Run = &evidence.HistoricalRunReferenceV1{RunID: "current-latest", Attempt: 2}
		},
		"unqualified": func(_ *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, c *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			c.LocalQualified = false
		},
		"private-owner-conflict": func(_ *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, c *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			c.BlockingReasons = []string{"actual_owner_conflict"}
		},
		"gap-pretends-verified": func(_ *evidence.HistoricalReferenceEntryV1, _ *evidence.Digest, c *HistoricalCandidate, _ *HistoricalCoordinatorBinding) {
			c.HistoricalGaps = []string{"missing_original_run_identity"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, d, c, b := entry.Clone(), content, coordinatorCloneCandidate(candidate), binding
			change(&e, &d, &c, &b)
			if err := casEntryMatchesCandidate(e, d, c, 42, b); !errors.Is(err, ErrHistoricalCASPersistence) {
				t.Fatal("changed reference admitted", err)
			}
		})
	}
}

func TestHistoricalCASPersistenceMissingOpaqueCapabilitiesNeverAuthorizes(t *testing.T) {
	var c *HistoricalCoordinator
	if p, err := c.SealHistoricalCASPersistencePage(t.Context(), nil, nil, nil, nil, nil); p != nil || !errors.Is(err, ErrHistoricalCASPersistence) {
		t.Fatal(err)
	}
	var p *HistoricalCASPersistencePage
	if s, err := p.BindApplied(t.Context(), nil, nil); s != nil || !errors.Is(err, ErrHistoricalCASPersistence) {
		t.Fatal(err)
	}
	var s *HistoricalCASAppliedPage
	if o, err := s.VerifyPersisted(t.Context(), nil, nil); o != nil || !errors.Is(err, ErrHistoricalCASPersistence) {
		t.Fatal(err)
	}
	for _, value := range []any{&HistoricalCASPersistencePage{}, &HistoricalCASAppliedPage{}, &HistoricalCASPersistenceObservation{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private cap serialized")
		}
	}
}

func TestHistoricalCASPersistenceReportIsLimitedAndDefensive(t *testing.T) {
	r := historicalCASPersistenceEmptyReport()
	if r.HostCommitResponseVerified || r.WholeRetirementPersistenceComplete || r.AICommandPersistenceComplete || r.BusinessClosureVerified || r.CASAuthorized || r.DropReady || len(r.Required) == 0 {
		t.Fatal("partial observation became full qualification")
	}
	// Only exercise synthetic report-clone integrity; no DB proof is minted.
	r.IndependentRawReadbackMatched = true
	o := &HistoricalCASPersistenceObservation{report: r}
	o.self = o
	o.seal = historicalCASObservationHash(r)
	view := o.Report()
	view.Required[0] = "caller changed"
	view.DropReady = true
	if got := o.Report(); got.Required[0] != r.Required[0] || got.DropReady {
		t.Fatal("mutable DTO changed cap")
	}
	copy := reflect.New(reflect.TypeOf(*o))
	copy.Elem().Set(reflect.ValueOf(o).Elem())
	if got := copy.Interface().(*HistoricalCASPersistenceObservation).Report(); got.IndependentRawReadbackMatched {
		t.Fatal("copied observation admitted")
	}
	o.report.PageSHA256 = "changed-private-page-binding"
	if got := o.Report(); got.IndependentRawReadbackMatched {
		t.Fatal("changed private report retained seal")
	}
}

func TestHistoricalCASPersistenceSelectorsKeepNullEmptyAndFalse(t *testing.T) {
	if casPersistenceSelectorFacts(nil) != nil {
		t.Fatal("nil selector normalized to empty")
	}
	empty := casPersistenceSelectorFacts(map[uint64]bool{})
	if empty == nil {
		t.Fatal("empty selector normalized to nil")
	}
	facts := casPersistenceSelectorFacts(map[uint64]bool{42: true, 43: false})
	if facts["42"] != true || facts["43"] != false || len(facts) != 2 {
		t.Fatal("original selector state hidden")
	}
}
