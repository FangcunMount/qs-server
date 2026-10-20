package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func transactionOutcomeForCodec(t *testing.T) *domainoutcome.Record {
	t.Helper()
	old, row := testCommittedReference(t, 9001, 42, "transaction-outcome-event")
	m, err := message.New(row.input())
	if err != nil {
		t.Fatal(err)
	}
	p, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mysql", Scope: row.Scope, OperationID: "operation", IdempotencyKey: "outcome/42:1",
		Branches: []transaction.BranchInput{{ID: "committed", Message: m, Wire: row.Payload, PhysicalTopic: "physical-transaction-topic", RouteRevision: "m3-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := p.Reference("committed", 1, "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(m), ref, old.BusinessBindingSHA256(), old.EvaluatedAt())
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{ID: old.ID(), OrgID: old.OrgID(), AssessmentID: old.AssessmentID(), TesteeID: old.TesteeID(), RunID: old.RunID(),
		Model: old.Model(), Runtime: old.Runtime(), InputSnapshotRef: old.InputSnapshotRef(), ReportInput: old.ReportInput(), Payload: old.Payload(), SchemaVersion: old.SchemaVersion(),
		EvaluatedAt: old.EvaluatedAt(), CommittedTransactionEvidence: proof})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestOutcomeJSONColumnRoundTripsBothEvidenceVersions(t *testing.T) {
	legacy, _ := testCommittedReference(t, 9002, 43, "historical-event")
	for _, record := range []*domainoutcome.Record{legacy, transactionOutcomeForCodec(t)} {
		po := outcomeToPO(record)
		raw, err := json.Marshal(po.CommittedEventEvidence)
		if err != nil {
			t.Fatal(err)
		}
		var decoded evidence.Record
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		po.CommittedEventEvidence = &decoded
		got, err := outcomeFromPO(po)
		if err != nil {
			t.Fatal(err)
		}
		if got.CommittedEventID() != record.CommittedEventID() || !bytes.Equal(got.Payload(), record.Payload()) || !bytes.Equal(got.ReportInput(), record.ReportInput()) {
			t.Fatal("column codec changed immutable outcome identity or original input/result bytes")
		}
		if (got.CommittedEventEvidence() == nil) != (record.CommittedEventEvidence() == nil) {
			t.Fatal("transaction reference was promoted to historical Outbox evidence")
		}
	}
}

func TestOutcomeJSONColumnRejectsMissingPairAndWrongStoreOwner(t *testing.T) {
	po := outcomeToPO(transactionOutcomeForCodec(t))
	po.CommittedEventID = nil
	if _, err := outcomeFromPO(po); err == nil {
		t.Fatal("missing canonical event pair accepted")
	}
	po = outcomeToPO(transactionOutcomeForCodec(t))
	ref, err := transaction.DecodeReference(po.CommittedEventEvidence.V2.TransactionReference)
	if err != nil {
		t.Fatal(err)
	}
	ref.Owner = "qs.mongo"
	po.CommittedEventEvidence.V2.TransactionReference, err = transaction.EncodeReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outcomeFromPO(po); err == nil {
		t.Fatal("Mongo witness referenced by a MySQL outcome")
	}
}

func TestOutcomeTransactionCannotSaveOutsideOriginalRootAndConstructorDoesNoDDL(t *testing.T) {
	pool, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		mock.ExpectClose()
		if err := pool.Close(); err != nil {
			t.Error(err)
		}
	}()
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	err = NewOutcomeRepository(db).Save(t.Context(), transactionOutcomeForCodec(t))
	if !errors.Is(err, hostmysql.ErrActiveTransactionRequired) {
		t.Fatal("transaction outcome saved without its original host root")
	}
	reader, err := NewConsistencyReadModelWithTransactions(db, transaction.Binding{Owner: "qs.mysql", StoreID: "host-primary", WriterEpoch: "m3-1"})
	if err != nil || reader == nil {
		t.Fatalf("inert primary authority constructor: %v", err)
	}
	if _, err := NewConsistencyReadModelWithTransactions(db, transaction.Binding{Owner: "qs.mongo", StoreID: "host-primary", WriterEpoch: "m3-1"}); err == nil {
		t.Fatal("wrong owner configured for primary MySQL reads")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal("constructor or rejected write performed database I/O")
	}
}
