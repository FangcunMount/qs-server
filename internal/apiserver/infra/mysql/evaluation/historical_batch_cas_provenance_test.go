package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

func TestSQLCASProvenanceRejectsZeroUnknownAndTypedNil(t *testing.T) {
	if p, err := SealSQLHistoricalCASProvenance(t.Context(), nil, nil); p != nil || !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("empty preparation admitted")
	}
	var tx *sql.Tx
	for _, pool := range []gorm.ConnPool{tx, &historicalUnknownPool{}} {
		g := &gorm.DB{Config: &gorm.Config{}, Statement: &gorm.Statement{ConnPool: pool}}
		batch := &SQLHistoricalOwnerBatch{cycle: &SQLHistoricalResponsibilityCycle{}, report: SQLHistoricalOwnerBatchReport{Complete: true}}
		plan := &SQLHistoricalBatchCASPlan{groups: []sqlHistoricalCASGroup{{table: "assessment"}}, attachments: []SQLHistoricalBatchAttachment{{}}}
		if p, err := SealSQLHistoricalCASProvenance(hostmysql.WithTx(t.Context(), g), plan, batch); p != nil || err == nil {
			t.Fatal("caller-set complete or unknown pool became provenance")
		}
	}
	var p *SQLHistoricalCASProvenance
	if _, err := p.Attachments(); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal(err)
	}
	if _, err := p.BindStatement(t.Context(), &SQLHistoricalBatchCASStatement{}); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal(err)
	}
	var bound *SQLHistoricalCASStatementBinding
	if err := bound.VerifyPersisted(t.Context(), nil); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal(err)
	}
	if r := bound.StatementReport(); r.StatementApplied || r.HostCommitVerified || r.BusinessClosureVerified || r.DropReady {
		t.Fatal("empty cap upgraded persistence")
	}
	for _, value := range []any{&SQLHistoricalCASProvenance{}, &SQLHistoricalCASStatementBinding{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSQLHistoricalFactsSerialization) {
			t.Fatal("private capability serialized")
		}
	}
	if b, err := SealSQLHistoricalCASReadBaseline(nil, &SQLHistoricalOwnerBatch{}); b != nil || !errors.Is(err, ErrSQLHistoricalCASProvenance) { //nolint:staticcheck // Negative test verifies that a nil context is rejected.
		t.Fatal("nil context admitted dependency baseline")
	}
	if err := (*SQLHistoricalCASReadBaseline)(nil).VerifyUnchanged(t.Context(), nil); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("nil dependency baseline admitted")
	}
}

// Synthetic package-private state tests only cloning/seal invalidation. It is
// never supplied to the actual snapshot constructor or used as DB evidence.
func TestSQLCASProvenanceCopiesAndAttachmentMutationRejected(t *testing.T) {
	plan := &SQLHistoricalBatchCASPlan{identity: "identity", server: "server", database: "database", before: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{"assessment": {{"id": evidence.String("42")}}}, schema: map[string]string{"assessment": "schema"}}, attachments: []SQLHistoricalBatchAttachment{{AssessmentID: 42, Entry: evidence.HistoricalReferenceEntryV1{EventID: "original", Proof: &evidence.EventEvidenceV1{BusinessBindingSHA256: "binding"}}}}}
	p := &SQLHistoricalCASProvenance{plan: plan, readPool: &historicalUnknownPool{}}
	p.self, p.seal = p, p.digest()
	if !p.intact() {
		t.Fatal("synthetic seal not formed")
	}
	copy := reflect.New(reflect.TypeOf(*p))
	copy.Elem().Set(reflect.ValueOf(p).Elem())
	if _, err := copy.Interface().(*SQLHistoricalCASProvenance).Attachments(); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("struct copy was a capability")
	}
	views, err := p.Attachments()
	if err != nil {
		t.Fatal(err)
	}
	views[0].Entry.Proof.BusinessBindingSHA256 = "caller mutation"
	if plan.attachments[0].Entry.Proof.BusinessBindingSHA256 != "binding" {
		t.Fatal("diagnostic view aliases proof")
	}
	plan.attachments[0].Entry.EventID = "changed"
	if p.intact() {
		t.Fatal("changed prepared source facts preserved seal")
	}
}

func TestSQLCASProvenanceEndingIsNotCommitClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sqlHistoricalEndedPool(ctx, &historicalUnknownPool{}); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("canceled/unknown epoch accepted")
	}
	var prepared *gorm.PreparedStmtTX
	if err := sqlHistoricalEndedPool(t.Context(), prepared); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("typed nil pool admitted")
	}
	// API reports never mark the host Commit response verified; an eventual
	// native test must exercise commit, rollback, active and unknown separately.
	if r := (*SQLHistoricalCASStatementBinding)(nil).StatementReport(); r.HostCommitVerified {
		t.Fatal("termination inferred Commit")
	}
}
