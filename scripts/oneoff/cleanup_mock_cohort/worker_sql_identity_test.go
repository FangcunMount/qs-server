package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
)

type workerIdentityTestConnector struct {
	mu    sync.Mutex
	conns []*workerIdentityTestConn
}

func (*workerIdentityTestConnector) Driver() driver.Driver { return workerIdentityTestDriver{} }
func (c *workerIdentityTestConnector) Connect(context.Context) (driver.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.conns) == 0 {
		return nil, errors.New("unexpected additional physical connection")
	}
	conn := c.conns[0]
	c.conns = c.conns[1:]
	return conn, nil
}

type workerIdentityTestDriver struct{}

func (workerIdentityTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("connector must be used")
}

type workerIdentityTestConn struct {
	identities                        []workerSQLBinding
	identityErr, rowsClose, closeErr  error
	rollbackErr                       error
	identityReads, userReads, begins  int
	execs, commits, rollbacks, closes int
}

func (*workerIdentityTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared query")
}
func (c *workerIdentityTestConn) Close() error { c.closes++; return c.closeErr }
func (c *workerIdentityTestConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *workerIdentityTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.begins++
	return workerIdentityTestTx{c}, nil
}
func (c *workerIdentityTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if query != workerSQLIdentityQuery {
		c.userReads++
		return &workerIdentityTestRows{columns: []string{"value"}, rows: [][]driver.Value{{int64(7)}}}, nil
	}
	index := c.identityReads
	c.identityReads++
	if c.identityErr != nil {
		return nil, c.identityErr
	}
	if index >= len(c.identities) {
		index = len(c.identities) - 1
	}
	if index < 0 {
		return nil, errors.New("missing modeled identity")
	}
	id := c.identities[index]
	return &workerIdentityTestRows{columns: []string{"@@server_uuid", "DATABASE()"}, rows: [][]driver.Value{{[]byte(id.uuid), id.database}}, closeErr: c.rowsClose}, nil
}
func (c *workerIdentityTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.execs++
	return driver.RowsAffected(1), nil
}

type workerIdentityTestTx struct{ conn *workerIdentityTestConn }

func (t workerIdentityTestTx) Commit() error { t.conn.commits++; return nil }
func (t workerIdentityTestTx) Rollback() error {
	t.conn.rollbacks++
	return t.conn.rollbackErr
}

type workerIdentityTestRows struct {
	columns  []string
	rows     [][]driver.Value
	next     int
	closeErr error
}

func (r *workerIdentityTestRows) Columns() []string { return r.columns }
func (r *workerIdentityTestRows) Close() error      { return r.closeErr }
func (r *workerIdentityTestRows) Next(values []driver.Value) error {
	if r.next == len(r.rows) {
		return io.EOF
	}
	copy(values, r.rows[r.next])
	r.next++
	return nil
}

func workerIdentityTestPool(t *testing.T, expected workerSQLBinding, conns ...*workerIdentityTestConn) *sql.DB {
	t.Helper()
	db := sql.OpenDB(workerSQLConnector{base: &workerIdentityTestConnector{conns: conns}, identity: expected})
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWorkerSQLRejectsSecondForeignPhysicalConnectionBeforeUserIO(t *testing.T) {
	expected := workerSQLBinding{"original-uuid", "owned"}
	for _, tc := range []struct {
		name string
		bad  workerSQLBinding
		tx   bool
	}{
		{"uuid_query", workerSQLBinding{"other-uuid", "owned"}, false},
		{"database_query", workerSQLBinding{"original-uuid", "other"}, false},
		{"uuid_transaction", workerSQLBinding{"other-uuid", "owned"}, true},
		{"database_transaction", workerSQLBinding{"original-uuid", "other"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := &workerIdentityTestConn{identities: []workerSQLBinding{expected}}
			foreign := &workerIdentityTestConn{identities: []workerSQLBinding{tc.bad}}
			db := workerIdentityTestPool(t, expected, original, foreign)
			pin, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = pin.Close() }()
			if tc.tx {
				_, err = (&liveStore{sqlPool: db, workerIdentity: expected}).beginGraphTx(t.Context(), nil)
			} else {
				var value int
				err = db.QueryRowContext(t.Context(), "SELECT 7").Scan(&value)
			}
			if err == nil || foreign.closes != 1 || foreign.begins != 0 || foreign.userReads != 0 || foreign.execs != 0 || original.identityReads != 1 || foreign.identityReads != 1 {
				t.Fatalf("foreign worker admitted: error=%v foreign=%+v", err, foreign)
			}
		})
	}
}

func TestWorkerSQLRejectsUnknownIdentityAndPreservesReleaseErrors(t *testing.T) {
	expected := workerSQLBinding{"original-uuid", "owned"}
	queryFailure, rowRelease, connectionRelease := errors.New("identity query unknown"), errors.New("identity rows release unknown"), errors.New("physical release unknown")
	for _, tc := range []struct {
		name string
		conn *workerIdentityTestConn
		want error
	}{
		{"query_error", &workerIdentityTestConn{identityErr: queryFailure, closeErr: connectionRelease}, queryFailure},
		{"row_release", &workerIdentityTestConn{identities: []workerSQLBinding{expected}, rowsClose: rowRelease, closeErr: connectionRelease}, rowRelease},
		{"foreign_release", &workerIdentityTestConn{identities: []workerSQLBinding{{"other", "owned"}}, closeErr: connectionRelease}, connectionRelease},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := workerIdentityTestPool(t, expected, tc.conn)
			if _, err := db.Conn(t.Context()); !errors.Is(err, tc.want) || !errors.Is(err, connectionRelease) || tc.conn.closes != 1 || tc.conn.userReads != 0 || tc.conn.begins != 0 {
				t.Fatalf("identity/release error was lost: %v", err)
			}
		})
	}
}

func TestWorkerSQLOriginalDriverAndBoundTransactionRemainUsable(t *testing.T) {
	expected := workerSQLBinding{"original-uuid", "owned"}
	original := &workerIdentityTestConn{identities: []workerSQLBinding{expected}}
	worker := &workerIdentityTestConn{identities: []workerSQLBinding{expected}}
	db := workerIdentityTestPool(t, expected, original, worker)
	pin, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pin.Close() }()
	if err = pin.Raw(func(conn any) error {
		if conn != original {
			return errors.New("bound connector replaced the original driver's interfaces")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var value int
	if err = db.QueryRowContext(t.Context(), "SELECT 7").Scan(&value); err != nil || value != 7 {
		t.Fatal("bound raw read failed", err)
	}
	tx, err := (&liveStore{sqlPool: db, workerIdentity: expected}).beginGraphTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(t.Context(), "fixed synthetic write"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil || worker.begins != 1 || worker.identityReads != 2 || worker.execs != 1 || worker.commits != 1 || worker.rollbacks != 0 {
		t.Fatalf("bound transaction failed: %v worker=%+v", err, worker)
	}
}

func TestWorkerSQLTransactionRechecksIdentityBeforeMutation(t *testing.T) {
	expected := workerSQLBinding{"original-uuid", "owned"}
	rollbackFailure := errors.New("transaction rollback release unknown")
	for _, changed := range []workerSQLBinding{{"foreign-uuid", "owned"}, {"original-uuid", "foreign-db"}} {
		worker := &workerIdentityTestConn{identities: []workerSQLBinding{expected, changed}, rollbackErr: rollbackFailure}
		db := workerIdentityTestPool(t, expected, worker)
		tx, err := (&liveStore{sqlPool: db, workerIdentity: expected}).beginGraphTx(t.Context(), nil)
		if tx != nil || !errors.Is(err, rollbackFailure) || worker.begins != 1 || worker.identityReads != 2 || worker.execs != 0 || worker.commits != 0 || worker.rollbacks != 1 {
			t.Fatalf("transaction changed identity admitted or release lost: %v worker=%+v", err, worker)
		}
	}
}
