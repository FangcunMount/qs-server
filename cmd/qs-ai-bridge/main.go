// qs-ai-bridge is an internal integration entry, not a participant API.
// Stage input must come from an authorized business caller. No user-auth bypass route is registered.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Keep machine-readable command results separate from asynchronous diagnostics.
	output := os.Stdout
	os.Stdout = os.Stderr
	if err := run(output); err != nil {
		fmt.Fprintln(os.Stderr, "AI bridge operation failed")
		os.Exit(1)
	}
}
func run(output io.Writer) error {
	return runArgs(output, os.Args[1:])
}

// runArgs rejects retired transport modes before opening a pool or reading keys.
func runArgs(output io.Writer, args []string) error {
	flags := flag.NewFlagSet("qs-ai-bridge", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", "", "projection or runtime-index-backfill")
	id := flags.String("request-id", "", "business request ID")
	batch := flags.Int("batch-size", 100, "runtime index backfill transaction size (1-500)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	switch *mode {
	case "stage-start", "stage-change", "relay", "receive":
		return errors.New("legacy AI transport retired; use the host MQ operation path")
	case "projection", "runtime-index-backfill":
	default:
		return errors.New("read or maintenance mode required")
	}
	ctx := context.Background()
	db, err := sql.Open("mysql", os.Getenv("QS_AI_BRIDGE_DSN"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(5)
	original := &store.Store{DB: db}
	if *mode == "projection" {
		value, err := original.Projection(ctx, *id)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(value)
	}
	for {
		count, err := original.BackfillRuntimeIndexes(ctx, *batch)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "indexed=%d\n", count); err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
	}
}
