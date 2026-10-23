// qs-retry-handoff transfers reviewed future responsibilities while old writers
// are stopped. It is a one-shot maintenance host, never a production runtime.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"golang.org/x/sys/unix"
)

type arguments struct {
	reviewFile, reviewSHA, database, receipt string
	binding                                  transaction.Binding
	apply, stopped                           bool
	timeout                                  time.Duration
}

func parseArguments(args []string) (arguments, error) {
	var a arguments
	f := flag.NewFlagSet("qs-retry-handoff", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&a.reviewFile, "review-file", "", "private canonical frozen selection")
	f.StringVar(&a.reviewSHA, "review-sha256", "", "independently reviewed raw file digest")
	f.StringVar(&a.binding.Owner, "owner", "", "configured original owner: qs.mysql or qs.mongo")
	f.StringVar(&a.binding.StoreID, "store-id", "", "configured original store identity")
	f.StringVar(&a.binding.WriterEpoch, "writer-epoch", "", "configured original writer epoch")
	f.StringVar(&a.database, "database", "", "explicit original database for apply")
	f.StringVar(&a.receipt, "receipt", "", "new private receipt path for apply")
	f.BoolVar(&a.apply, "apply", false, "apply through original DB transactions")
	f.BoolVar(&a.stopped, "all-legacy-writers-stopped-and-admission-closed", false, "operator attestation, not a stop mechanism or production approval")
	f.DurationVar(&a.timeout, "timeout", 2*time.Minute, "total apply deadline, at most 5m")
	if f.Parse(args) != nil || f.NArg() != 0 || a.reviewFile == "" || a.reviewSHA == "" ||
		!a.binding.Valid() || (a.binding.Owner != "qs.mysql" && a.binding.Owner != "qs.mongo") ||
		a.timeout <= 0 || a.timeout > 5*time.Minute ||
		(a.apply && (a.database == "" || a.receipt == "" || !a.stopped)) ||
		(!a.apply && (a.database != "" || a.receipt != "" || a.stopped)) {
		return a, errors.New("explicit bounded reviewed handoff arguments required")
	}
	return a, nil
}

func readReview(path, digest string) (model.HandoffReview, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return model.HandoffReview{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 1<<20 {
		return model.HandoffReview{}, errors.New("bounded private regular review file required")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return model.HandoffReview{}, err
	}
	return model.DecodeHandoffReview(raw, digest)
}

type executor interface {
	Apply(context.Context, model.HandoffReview, app.HandoffPreconditions) ([]app.HandoffOutcome, error)
}

type openHost func(context.Context, arguments, func(string) string) (executor, func() error, error)

func run(parent context.Context, args []string, getenv func(string) string, output io.Writer, open openHost) (resultErr error) {
	a, err := parseArguments(args)
	if err != nil {
		return err
	}
	review, err := readReview(a.reviewFile, a.reviewSHA)
	if err != nil {
		return err
	}
	if review.Binding() != a.binding || review.SourceSHA() != version.GitCommit {
		return errors.New("review differs from explicit binding or compiled source")
	}
	if !a.apply {
		return json.NewEncoder(output).Encode(struct {
			Status            string `json:"status"`
			Plans             int    `json:"plans"`
			CutoverAuthorized bool   `json:"cutover_authorized"`
		}{"offline_review_valid", len(review.Plans()), false})
	}
	// Reserve and fsync private evidence BEFORE any database connection. Existing
	// receipts/symlinks are refused. A crash leaves a started, never successful,
	// receipt; exact DB facts and same-plan retry remain authoritative.
	receipt, err := os.OpenFile(a.receipt, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, receipt.Close()) }()
	digest := review.Digest()
	header := struct {
		Schema            string              `json:"schema"`
		Status            string              `json:"status"`
		SourceSHA         string              `json:"source_sha"`
		ReviewSHA         string              `json:"review_sha256"`
		Actor             string              `json:"actor"`
		Reason            string              `json:"reason"`
		Binding           transaction.Binding `json:"binding"`
		Database          string              `json:"database"`
		StoppedAttested   bool                `json:"writers_stopped_and_admission_closed_attested"`
		CutoverAuthorized bool                `json:"cutover_authorized"`
	}{"m3-future-handoff-receipt/v1", "started", version.GitCommit, hex.EncodeToString(digest[:]), review.Actor(), review.Reason(), a.binding, a.database, a.stopped, false}
	if err := json.NewEncoder(receipt).Encode(header); err != nil {
		return err
	}
	if err := receipt.Sync(); err != nil {
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
	outcomes, applyErr := host.Apply(ctx, review, app.HandoffPreconditions{
		AdmissionClosed: a.stopped, AllLegacyWritersStopped: a.stopped,
		ExpectedSourceSHA: version.GitCommit, ReviewedDigest: digest,
	})
	closeErr := closeHost()
	closed = true
	status := "handoff_complete"
	if applyErr != nil || closeErr != nil {
		status = "handoff_incomplete"
	}
	final := struct {
		Status            string               `json:"status"`
		Outcomes          []app.HandoffOutcome `json:"outcomes"`
		CutoverAuthorized bool                 `json:"cutover_authorized"`
	}{status, outcomes, false}
	writeErr := json.NewEncoder(receipt).Encode(final)
	syncErr := receipt.Sync()
	if err := errors.Join(applyErr, closeErr, writeErr, syncErr); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Status            string `json:"status"`
		Confirmed         int    `json:"confirmed"`
		CutoverAuthorized bool   `json:"cutover_authorized"`
	}{status, len(outcomes), false})
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, openOriginalHost); err != nil {
		// Database/driver errors and private file contents can contain secrets.
		fmt.Fprintln(os.Stderr, "offline future handoff refused or incomplete; preserve private receipt and original responsibilities")
		os.Exit(1)
	}
}
