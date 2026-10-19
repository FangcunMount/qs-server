// mysql_tls_probe reads only connection identity and TLS session metadata.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
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
	connector, err := database.NewMySQLConnector(&database.MySQLConfig{
		Host: cfg.Addr, Username: cfg.User, Password: cfg.Passwd, Database: cfg.DBName,
		TLSMode: os.Getenv("MYSQL_TLS_MODE"), TLSCAFile: os.Getenv("MYSQL_TLS_CA_FILE"),
		Location: os.Getenv("MYSQL_LOCATION"), SessionTimeZone: os.Getenv("MYSQL_SESSION_TIME_ZONE"),
	})
	if os.Getenv("MYSQL_PROBE_MAINTENANCE") == "1" {
		connector, err = mysqltls.NewConnector(cfg)
	}
	if err != nil {
		return err
	}
	pool := sql.OpenDB(connector)
	defer func() { _ = pool.Close() }()
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
		detail := map[string]any{"mysql_tls_probe": "failed", "error_type": fmt.Sprintf("%T", err)}
		var sqlError *mysql.MySQLError
		var certError *tls.CertificateVerificationError
		var unknown x509.UnknownAuthorityError
		if errors.As(err, &sqlError) {
			detail["mysql_error_code"] = sqlError.Number
		}
		if errors.As(err, &certError) {
			detail["certificate_error"] = certError.Err.Error()
		}
		if errors.As(err, &unknown) {
			detail["certificate_error"] = "unknown authority"
		}
		_ = json.NewEncoder(os.Stderr).Encode(detail)
		os.Exit(1)
	}
}
