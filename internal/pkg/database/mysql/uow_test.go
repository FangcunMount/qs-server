package mysql

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestUnitOfWorkNilDBFailsClosed(t *testing.T) {
	t.Parallel()

	called := false
	err := NewUnitOfWork(nil).WithinTransaction(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrUnitOfWorkUnavailable) {
		t.Fatalf("WithinTransaction() error = %v, want ErrUnitOfWorkUnavailable", err)
	}
	if called {
		t.Fatal("callback was called without a database")
	}
}

func TestWithTxBridgeRequiresTransaction(t *testing.T) {
	t.Parallel()

	tx := &gorm.DB{}
	ctx := WithTx(context.Background(), tx)
	got, err := RequireTx(ctx)
	if err != nil {
		t.Fatalf("RequireTx() error = %v", err)
	}
	if got != tx {
		t.Fatal("RequireTx() did not return context transaction")
	}

	_, err = RequireTx(context.Background())
	if !errors.Is(err, ErrActiveTransactionRequired) {
		t.Fatalf("RequireTx() error = %v, want ErrActiveTransactionRequired", err)
	}
}
