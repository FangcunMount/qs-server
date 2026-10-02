//go:build integration && (m4_07_old_chain || m4_07_new_chain)

package transaction

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// This connector measures client-side command round trips in disposable tests.
// It records only Outbox table and command class, never SQL parameters or payloads.
func m407OpenMonitoredMySQL(dsn string) (*gorm.DB, func() error, error) {
	path := os.Getenv("RM_QS_M407_MYSQL_COMMAND_METRICS")
	if path == "" {
		db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
		return db, func() error { return nil }, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	connector, err := (mysqldriver.MySQLDriver{}).OpenConnector(dsn)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	monitor := &m407SQLMonitor{encoder: json.NewEncoder(file)}
	sqlDB := sql.OpenDB(m407Connector{Connector: connector, monitor: monitor})
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		file.Close()
		return nil, nil, err
	}
	return db, file.Close, nil
}

type m407SQLMonitor struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

func (m *m407SQLMonitor) record(query, operation string, start time.Time, err error) {
	text := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(query), "`", ""))
	table := ""
	for _, name := range []string{"domain_event_outbox", "rm_outbox"} {
		if strings.Contains(text, "from "+name) || strings.Contains(text, "into "+name) || strings.Contains(text, "update "+name) || strings.Contains(text, "delete "+name) {
			table = name
			break
		}
	}
	if table == "" {
		return
	}
	class := strings.ToLower(strings.Fields(text)[0])
	elapsed := time.Since(start).Nanoseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.encoder.Encode(struct {
		Table      string `json:"table"`
		Class      string `json:"class"`
		Operation  string `json:"operation"`
		DurationNS int64  `json:"duration_ns"`
		Error      bool   `json:"error"`
	}{table, class, operation, elapsed, err != nil})
}

type m407Connector struct {
	driver.Connector
	monitor *m407SQLMonitor
}

func (c m407Connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &m407Conn{Conn: conn, monitor: c.monitor}, nil
}

type m407Conn struct {
	driver.Conn
	monitor *m407SQLMonitor
}

func (c *m407Conn) Prepare(query string) (driver.Stmt, error) {
	start := time.Now()
	stmt, err := c.Conn.Prepare(query)
	c.monitor.record(query, "prepare", start, err)
	if err != nil {
		return nil, err
	}
	return &m407Stmt{Stmt: stmt, query: query, monitor: c.monitor}, nil
}

func (c *m407Conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		start := time.Now()
		stmt, err := preparer.PrepareContext(ctx, query)
		c.monitor.record(query, "prepare", start, err)
		if err != nil {
			return nil, err
		}
		return &m407Stmt{Stmt: stmt, query: query, monitor: c.monitor}, nil
	}
	return c.Prepare(query)
}

func (c *m407Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		return beginner.BeginTx(ctx, opts)
	}
	return c.Conn.Begin()
}

func (c *m407Conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if executor, ok := c.Conn.(driver.ExecerContext); ok {
		start := time.Now()
		result, err := executor.ExecContext(ctx, query, args)
		if err != driver.ErrSkip {
			c.monitor.record(query, "exec", start, err)
		}
		return result, err
	}
	return nil, driver.ErrSkip
}

func (c *m407Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if querier, ok := c.Conn.(driver.QueryerContext); ok {
		start := time.Now()
		rows, err := querier.QueryContext(ctx, query, args)
		if err != driver.ErrSkip {
			c.monitor.record(query, "query", start, err)
		}
		return rows, err
	}
	return nil, driver.ErrSkip
}

func (c *m407Conn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

func (c *m407Conn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.Conn.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

func (c *m407Conn) IsValid() bool {
	if validator, ok := c.Conn.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

func (c *m407Conn) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := c.Conn.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return driver.ErrSkip
}

type m407Stmt struct {
	driver.Stmt
	query   string
	monitor *m407SQLMonitor
}

func (s *m407Stmt) Exec(args []driver.Value) (driver.Result, error) {
	start := time.Now()
	result, err := s.Stmt.Exec(args)
	s.monitor.record(s.query, "exec", start, err)
	return result, err
}

func (s *m407Stmt) Query(args []driver.Value) (driver.Rows, error) {
	start := time.Now()
	rows, err := s.Stmt.Query(args)
	s.monitor.record(s.query, "query", start, err)
	return rows, err
}

func (s *m407Stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if executor, ok := s.Stmt.(driver.StmtExecContext); ok {
		start := time.Now()
		result, err := executor.ExecContext(ctx, args)
		s.monitor.record(s.query, "exec", start, err)
		return result, err
	}
	return nil, driver.ErrSkip
}

func (s *m407Stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if querier, ok := s.Stmt.(driver.StmtQueryContext); ok {
		start := time.Now()
		rows, err := querier.QueryContext(ctx, args)
		s.monitor.record(s.query, "query", start, err)
		return rows, err
	}
	return nil, driver.ErrSkip
}

func (s *m407Stmt) CheckNamedValue(value *driver.NamedValue) error {
	if checker, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(value)
	}
	return driver.ErrSkip
}
