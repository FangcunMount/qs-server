package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
)

const workerSQLIdentityQuery = "SELECT @@server_uuid,DATABASE()"

type workerSQLBinding struct{ uuid, database string }

func (b workerSQLBinding) valid() bool { return b.uuid != "" && b.database != "" }

// Preserve the original mysql driver connection and all of its optional
// interfaces, but admit it to this tool's pool only after its own identity read.
type workerSQLConnector struct {
	base     driver.Connector
	identity workerSQLBinding
}

func (c workerSQLConnector) Driver() driver.Driver { return c.base.Driver() }
func (c workerSQLConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if c.base == nil || !c.identity.valid() {
		return nil, errors.New("fixed worker SQL identity required before connecting")
	}
	conn, err := c.base.Connect(ctx)
	if err != nil {
		if conn != nil {
			err = errors.Join(err, conn.Close())
		}
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("worker SQL driver returned no connection")
	}
	if err = c.check(ctx, conn); err != nil {
		return nil, errors.Join(errors.New("worker SQL physical connection identity rejected"), err, conn.Close())
	}
	return conn, nil
}

func (c workerSQLConnector) check(ctx context.Context, conn driver.Conn) (err error) {
	q, ok := conn.(driver.QueryerContext)
	if !ok {
		return errors.New("worker SQL driver must support direct bounded identity query")
	}
	rows, err := q.QueryContext(ctx, workerSQLIdentityQuery, nil)
	if err != nil {
		if rows != nil {
			err = errors.Join(err, rows.Close())
		}
		return err
	}
	if rows == nil {
		return errors.New("worker SQL identity returned no rows")
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	if len(rows.Columns()) != 2 {
		return errors.New("worker SQL identity column shape changed")
	}
	values := make([]driver.Value, 2)
	if err = rows.Next(values); err != nil {
		return err
	}
	text := func(v driver.Value) string {
		switch x := v.(type) {
		case string:
			return x
		case []byte:
			return string(x)
		}
		return ""
	}
	if text(values[0]) != c.identity.uuid || text(values[1]) != c.identity.database {
		return errors.New("worker SQL server UUID or database changed")
	}
	if err = rows.Next(values); !errors.Is(err, io.EOF) {
		return errors.Join(errors.New("worker SQL identity must return exactly one row"), err)
	}
	return nil
}

func checkWorkerSQLTransaction(ctx context.Context, tx *sql.Tx, expected workerSQLBinding) error {
	if !expected.valid() {
		return errors.New("fixed worker SQL transaction identity required")
	}
	var uuid, database string
	if err := tx.QueryRowContext(ctx, workerSQLIdentityQuery).Scan(&uuid, &database); err != nil {
		return err
	}
	if uuid != expected.uuid || database != expected.database {
		return errors.New("worker SQL transaction server UUID or database changed")
	}
	return nil
}
