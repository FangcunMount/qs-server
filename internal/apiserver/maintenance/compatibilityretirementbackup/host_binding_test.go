package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mongodriver "go.mongodb.org/mongo-driver/mongo"
)

func TestHostArchiveBindingsKeepInventoryAndRecoveryProjectionsSeparate(t *testing.T) {
	inventory, recovery, mongo := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	a := &Archive{data: manifest{SQLRecoveryNonTargetHash: recovery, Inventory: inventoryFixtureForHost(inventory, mongo)}}
	var expected [4]HostObjectBinding
	for i, s := range a.data.Inventory.Targets {
		expected[i] = HostObjectBinding{Database: s.Database, Name: s.Name, Kind: s.Kind, IdentityHash: s.IdentityHash, SchemaHash: s.SchemaHash, DataHash: s.DataHash, Records: s.Records}
	}
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, recovery, mongo) != nil {
		t.Fatal("separate approved projections rejected")
	}
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, inventory, mongo) == nil {
		t.Fatal("inventory projection adopted for recovery")
	}
	a.data.SQLRecoveryNonTargetHash = ""
	if VerifyHostArchiveObjects(context.Background(), a, expected, 99, 38, inventory, recovery, mongo) == nil {
		t.Fatal("old archive without projection adopted")
	}
	if VerifyHostArchiveObjects(context.Background(), new(Archive), expected, 99, 38, inventory, recovery, mongo) == nil {
		t.Fatal("zero archive accepted")
	}
	t.Run("snapshot_requires_actual_host_epoch_and_keeps_borrowed_tx", func(t *testing.T) {
		db, mock, e := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		tx, e := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = tx.Rollback() }()
		client, e := mongodriver.NewClient() //nolint:staticcheck // Offline bare handle exercises rejection without opening a native connection.
		if e != nil {
			t.Fatal(e)
		}
		borrowed := BorrowedSources{SQL: tx, Mongo: client.Database("borrowed_snapshot_guard_test")}
		// A requested SQL mode and bare Mongo handle are not a paired epoch.
		if VerifyHostOriginalSnapshotSources(t.Context(), a, borrowed) != ErrNonTargetData {
			t.Fatal("bare read handles supplied a snapshot proof")
		}
		cancelled, cancel := context.WithCancel(t.Context())
		cancel()
		if VerifyHostOriginalSnapshotSources(cancelled, a, borrowed) != ErrNonTargetData {
			t.Fatal("expired host scope supplied a snapshot proof")
		}
		mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))
		var value int
		if tx.QueryRowContext(t.Context(), "SELECT 1").Scan(&value) != nil || value != 1 {
			t.Fatal("rejected snapshot guard ended the host's borrowed transaction")
		}
		mock.ExpectRollback()
		if tx.Rollback() != nil || mock.ExpectationsWereMet() != nil {
			t.Fatal("host transaction cleanup changed")
		}
	})
}
func inventoryFixtureForHost(sqlHash, mongoHash string) inventory {
	r := inventory{Bindings: map[string]Binding{"mysql": {Version: 99, NonTargetHash: sqlHash}, "mongodb": {Version: 38, NonTargetHash: mongoHash}}}
	for i, name := range targetNames {
		database, kind := "mysql", "base_table"
		if i == 3 {
			database, kind = "mongodb", "collection"
		}
		r.Targets = append(r.Targets, SourceSnapshot{Database: database, Name: name, Kind: kind, IdentityHash: strings.Repeat("d", 64), SchemaHash: strings.Repeat("e", 64), DataHash: strings.Repeat("f", 64)})
	}
	return r
}
