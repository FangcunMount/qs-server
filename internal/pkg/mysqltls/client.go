// Package mysqltls applies the production connection policy to operator tools.
package mysqltls

import (
	"database/sql"
	"database/sql/driver"
	"os"

	"github.com/FangcunMount/component-base/pkg/database"
	mysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// NewConnector preserves caller-owned charset, timezone, and timeout options.
func NewConnector(cfg *mysql.Config) (driver.Connector, error) {
	mode, ca := os.Getenv("MYSQL_TLS_MODE"), os.Getenv("MYSQL_TLS_CA_FILE")
	if mode == "" {
		mode, ca = os.Getenv("QS_APISERVER_MYSQL_TLS_MODE"), os.Getenv("QS_APISERVER_MYSQL_TLS_CA_FILE")
	}
	if mode == "" {
		mode, ca = os.Getenv("QS_WORKER_MYSQL_TLS_MODE"), os.Getenv("QS_WORKER_MYSQL_TLS_CA_FILE")
	}
	if err := database.ConfigureMySQLTLS(cfg, mode, ca); err != nil {
		return nil, err
	}
	return mysql.NewConnector(cfg)
}

func Open(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	connector, err := NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}

func OpenGORM(dsn string, options *gorm.Config) (*gorm.DB, error) {
	pool, err := Open(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool}), options)
	if err != nil {
		_ = pool.Close()
	}
	return db, err
}
