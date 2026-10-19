// mysql_tls_probe reads only connection identity and TLS session metadata.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	mysql "github.com/go-sql-driver/mysql"
)

type evidence struct {
	Endpoint   string `json:"endpoint"`
	Database   string `json:"database"`
	UUID       string `json:"server_uuid"`
	Connection uint64 `json:"connection_id"`
	Cipher     string `json:"ssl_cipher"`
	Version    string `json:"ssl_version"`
}

func run() error {
	if os.Getenv("MYSQL_TLS_MODE") != "VERIFY_IDENTITY" {
		return errors.New("VERIFY_IDENTITY is required")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.DBName = os.Getenv("MYSQL_USERNAME"), os.Getenv("MYSQL_PASSWORD"), os.Getenv("MYSQL_DATABASE")
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(os.Getenv("MYSQL_HOST"), os.Getenv("MYSQL_PORT"))
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 8*time.Second, 8*time.Second, 8*time.Second
	connector, err := mysqltls.NewConnector(cfg)
	if err != nil {
		return err
	}
	pool := sql.OpenDB(connector)
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var previous uint64
	for i := 0; i < 2; i++ {
		conn, err := pool.Conn(ctx)
		if err != nil {
			return err
		}
		item := evidence{Endpoint: cfg.Addr}
		err = conn.QueryRowContext(ctx, "SELECT @@server_uuid,DATABASE(),CONNECTION_ID()").Scan(&item.UUID, &item.Database, &item.Connection)
		if err == nil {
			var key string
			err = conn.QueryRowContext(ctx, "SHOW SESSION STATUS LIKE 'Ssl_cipher'").Scan(&key, &item.Cipher)
			if err == nil {
				err = conn.QueryRowContext(ctx, "SHOW SESSION STATUS LIKE 'Ssl_version'").Scan(&key, &item.Version)
			}
		}
		closeErr := conn.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if item.Database != cfg.DBName || item.Cipher == "" || (item.Version != "TLSv1.2" && item.Version != "TLSv1.3") || item.Connection == previous {
			return errors.New("connection identity or TLS session verification failed")
		}
		previous = item.Connection
		if err = json.NewEncoder(os.Stdout).Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "MySQL TLS probe failed; no credentials or raw SQL errors emitted")
		os.Exit(1)
	}
}
