package actor

import (
	"context"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"gorm.io/gorm"
)

func TestTesteeStoreMapperRoundTrip(t *testing.T) {
	subject := testee.NewTestee(7, "测试", testee.Gender(0), nil)
	id := uint64(42)
	subject.RestoreStore(&id, 5)
	mapper := NewTesteeMapper()
	restored := mapper.ToDomain(mapper.ToPO(subject))
	if restored.StoreID() == nil || *restored.StoreID() != 42 || restored.StoreVersion() != 5 {
		t.Fatal("ownership lost during persistence round trip")
	}
	subject.RestoreStore(nil, 1)
	restored = mapper.ToDomain(mapper.ToPO(subject))
	if restored.StoreID() != nil || restored.StoreVersion() != 1 {
		t.Fatal("unassigned state not preserved")
	}
}

func TestOrdinaryTesteeUpdateCannotOverwriteStoreOwnership(t *testing.T) {
	db := newDryRunActorDB(t).Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true})
	var statement string
	if err := db.Callback().Update().After("gorm:update").Register("capture_testee_store", func(tx *gorm.DB) { statement = tx.Statement.SQL.String() }); err != nil {
		t.Fatal(err)
	}
	subject := testee.NewTestee(7, "测试", testee.Gender(0), nil)
	subject.SetID(3)
	id := uint64(42)
	subject.RestoreStore(&id, 5)
	if err := NewTesteeRepository(db).Update(context.Background(), subject); err != nil {
		t.Fatal(err)
	}
	if statement == "" {
		t.Fatal("no update generated")
	}
	for _, field := range []string{"store_id", "store_version", "org_id", "created_at"} {
		if strings.Contains(statement, "`"+field+"`=") {
			t.Fatalf("ordinary update overwrites %s: %s", field, statement)
		}
	}
}
