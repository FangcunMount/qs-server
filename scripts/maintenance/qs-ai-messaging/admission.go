// admission operates only the host's persistent admission row.
// It never installs schema, starts a relay, migrates records or calls a model.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	"os"
	"strconv"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/go-sql-driver/mysql"
)

func runAdmission(action, expected string) (resultErr error) {
	revision, err := admissionRevision(action, expected)
	if err != nil {
		return err
	}
	cfg, err := mysql.ParseDSN(os.Getenv("QS_AI_MESSAGING_CONTROL_DSN"))
	if err != nil || cfg.DBName == "" {
		return errors.New("explicit host database required")
	}
	cfg.ParseTime, cfg.Loc = true, time.UTC
	db, err := mysqltls.Open(cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: action == "inspect"})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	gate := store.MessagingAdmission{}
	var state store.MessagingAdmissionState
	if action == "inspect" {
		state, err = gate.Inspect(ctx, tx)
	} else {
		state, err = gate.Set(ctx, tx, action == "close", revision)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		SourceSHA string                        `json:"source_sha"`
		Action    string                        `json:"action"`
		State     store.MessagingAdmissionState `json:"state"`
	}{sourceSHA, action, state})
}

func admissionRevision(action, expected string) (uint64, error) {
	if action != "inspect" && action != "close" && action != "open" {
		return 0, errors.New("unsupported action")
	}
	var revision uint64
	if action != "inspect" {
		var err error
		revision, err = strconv.ParseUint(expected, 10, 64)
		if err != nil || strconv.FormatUint(revision, 10) != expected {
			return 0, errors.New("reviewed revision required")
		}
	} else if expected != "" {
		return 0, errors.New("inspect does not change revision")
	}
	return revision, nil
}
