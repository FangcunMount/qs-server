package plan

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FangcunMount/component-base/pkg/event"
	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"testing"
)

type commitPublishSpy struct {
	calls int
	inTx  bool
}

type failingCommitPublisher struct {
	calls int
	inTx  bool
}

func (p *failingCommitPublisher) Publish(ctx context.Context, _ event.DomainEvent) error {
	p.calls++
	_, p.inTx = gormuow.TxFromContext(ctx)
	return errors.New("broker unavailable after commit")
}

func (p *failingCommitPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

func (s *commitPublishSpy) Publish(ctx context.Context, _ event.DomainEvent) error {
	s.calls++
	_, s.inTx = gormuow.TxFromContext(ctx)
	return nil
}
func (s *commitPublishSpy) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := s.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}
func TestPlanNotificationsWaitForOutermostCommit(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			mock.ExpectClose()
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		}()
		orm, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectBegin()
		if rollback {
			mock.ExpectRollback()
		} else {
			mock.ExpectCommit()
		}
		spy := &commitPublishSpy{}
		publisher := NewCommitPublisher(spy)
		uow := gormuow.NewUnitOfWork(orm)
		failure := errors.New("rollback")
		err = uow.WithinTransaction(context.Background(), func(ctx context.Context) error {
			if err := uow.WithinTransaction(ctx, func(inner context.Context) error { return publisher.PublishAll(inner, []event.DomainEvent{nil, nil}) }); err != nil {
				return err
			}
			if spy.calls != 0 {
				t.Fatal("published before outer commit")
			}
			if rollback {
				return failure
			}
			return nil
		})
		if rollback {
			if !errors.Is(err, failure) || spy.calls != 0 {
				t.Fatal("rolled back transaction published")
			}
		} else if err != nil || spy.calls != 2 || spy.inTx {
			t.Fatalf("commit publication: %v %+v", err, spy)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanNotificationFailureAfterCommitDoesNotRollbackBusinessWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	orm, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE assessment_task SET status").WithArgs("opened", 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	spy := &failingCommitPublisher{}
	uow := gormuow.NewUnitOfWork(orm)
	err = uow.WithinTransaction(context.Background(), func(ctx context.Context) error {
		tx, ok := gormuow.TxFromContext(ctx)
		if !ok {
			return errors.New("business write has no active transaction")
		}
		if err := tx.Exec("UPDATE assessment_task SET status = ? WHERE id = ?", "opened", 42).Error; err != nil {
			return err
		}
		return NewCommitPublisher(spy).Publish(ctx, nil)
	})
	if err != nil {
		t.Fatalf("post-commit broker failure changed committed business result: %v", err)
	}
	if spy.calls != 1 || spy.inTx {
		t.Fatalf("publisher calls = %d, inTx = %t; want one call outside the transaction", spy.calls, spy.inTx)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
