package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"golang.org/x/sys/unix"
)

var errInventoryPending = errors.New("original ledger responsibilities remain")

type inspector interface {
	Inspect(context.Context, []model.Frozen) (model.LegacyInventory, error)
}

type historyInspector interface {
	InspectHistory(context.Context, []model.Frozen) (model.LegacyInventory, error)
}

func runInventory(parent context.Context, a arguments, getenv func(string) string, output io.Writer, open openHost) (resultErr error) {
	source, err := hex.DecodeString(version.GitCommit)
	if err != nil || len(source) != 20 || hex.EncodeToString(source) != version.GitCommit {
		return errors.New("compiled source identity required for inventory")
	}
	var selected []model.Frozen
	if a.reviewFile != "" {
		review, err := readReview(a.reviewFile, a.reviewSHA)
		if err != nil {
			return err
		}
		if review.Binding() != a.binding || review.SourceSHA() != version.GitCommit {
			return errors.New("inventory selection differs from configured original binding or source")
		}
		selected = review.Plans()
	}
	f, err := os.OpenFile(a.receipt, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	schema := "m3-future-handoff-inventory/v1"
	if a.history {
		schema = "m5-mongo-handoff-history-inventory/v1"
	}
	header := struct {
		Schema            string              `json:"schema"`
		Status            string              `json:"status"`
		SourceSHA         string              `json:"source_sha"`
		Binding           transaction.Binding `json:"binding"`
		Database          string              `json:"database"`
		Selected          int                 `json:"selected_handoffs"`
		ReadOnly          bool                `json:"read_only"`
		CutoverAuthorized bool                `json:"cutover_authorized"`
	}{schema, "started", version.GitCommit, a.binding, a.database, len(selected), true, false}
	if err := json.NewEncoder(f).Encode(header); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, a.timeout)
	defer cancel()
	host, closeHost, err := open(ctx, a, getenv)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, closeHost())
		}
	}()
	started := time.Now().UTC()
	var snapshot model.LegacyInventory
	var inspectErr error
	if a.history {
		reader, ok := host.(historyInspector)
		if !ok {
			return errors.New("host does not implement Mongo history inspection")
		}
		snapshot, inspectErr = reader.InspectHistory(ctx, selected)
	} else {
		reader, ok := host.(inspector)
		if !ok {
			return errors.New("host does not implement original-ledger inspection")
		}
		snapshot, inspectErr = reader.Inspect(ctx, selected)
	}
	if inspectErr == nil && snapshot.Binding != a.binding {
		inspectErr = transaction.ErrBinding
	}
	closeErr := closeHost()
	closed = true
	status := "old_responsibilities_accounted"
	if !snapshot.ResponsibilitiesAccounted {
		status = "old_responsibilities_present"
	}
	if a.history {
		status = "history_responsibilities_accounted"
		if !snapshot.ResponsibilitiesAccounted {
			status = "history_responsibilities_present"
		}
	}
	var verified *model.LegacyInventory
	if inspectErr == nil {
		verified = &snapshot
	}
	if inspectErr != nil || closeErr != nil {
		status = "inventory_unavailable"
	}
	final := struct {
		Status            string                 `json:"status"`
		StartedAt         time.Time              `json:"started_at"`
		CompletedAt       time.Time              `json:"completed_at"`
		Inventory         *model.LegacyInventory `json:"inventory,omitempty"`
		CutoverAuthorized bool                   `json:"cutover_authorized"`
	}{status, started, time.Now().UTC(), verified, false}
	writeErr := json.NewEncoder(f).Encode(final)
	syncErr := f.Sync()
	if err := errors.Join(inspectErr, closeErr, writeErr, syncErr); err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(struct {
		Status            string `json:"status"`
		Total             uint64 `json:"total"`
		Active            uint64 `json:"active"`
		UnverifiedHeld    uint64 `json:"unverified_held"`
		CutoverAuthorized bool   `json:"cutover_authorized"`
	}{status, snapshot.Total, snapshot.Active, snapshot.UnverifiedHeld, false}); err != nil {
		return err
	}
	if !snapshot.ResponsibilitiesAccounted {
		return errInventoryPending
	}
	return nil
}
