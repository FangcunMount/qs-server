package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/sys/unix"
)

type initialBindingArguments struct {
	action, plan, reviewed, operation, owner, store, epoch string
	writersExcluded                                        bool
}

func registerInitialBindingFlags(f *flag.FlagSet, b *initialBindingArguments) {
	f.StringVar(&b.action, "binding-action", "", "preview, apply or verify initial owner; never runs DDL")
	f.StringVar(&b.plan, "binding-plan", "", "private original plan file")
	f.StringVar(&b.reviewed, "binding-reviewed-sha256", "", "independently approved original raw digest")
	f.StringVar(&b.operation, "binding-operation-id", "", "explicit operation identity for preview")
	f.StringVar(&b.owner, "binding-owner", "", "qs.mysql or qs.mongo for preview")
	f.StringVar(&b.store, "binding-store-id", "", "explicit original SDK store identity for preview")
	f.StringVar(&b.epoch, "binding-writer-epoch", "", "frozen original writer epoch for preview")
	f.BoolVar(&b.writersExcluded, "binding-writers-excluded", false, "attest original writers excluded; not proof")
}
func (b initialBindingArguments) validate(apply bool, approval, sha string) error {
	if b.action == "" {
		if b.plan != "" || b.reviewed != "" || b.operation != "" || b.owner != "" || b.store != "" || b.epoch != "" || b.writersExcluded {
			return errors.New("binding fields require a separate binding action")
		}
		return nil
	}
	if apply || approval != "" || sha != "" || b.plan == "" {
		return errors.New("initial binding cannot share migration action or approval")
	}
	switch b.action {
	case "preview":
		if b.reviewed != "" || b.writersExcluded || b.operation == "" || b.store == "" || b.epoch == "" || (b.owner != "qs.mysql" && b.owner != "qs.mongo") {
			return errors.New("explicit new original binding preview required")
		}
	case "apply", "verify":
		if b.reviewed == "" || b.operation != "" || b.owner != "" || b.store != "" || b.epoch != "" || (b.action == "apply" && !b.writersExcluded) {
			return errors.New("reviewed original binding plan and writer attestation required")
		}
	default:
		return errors.New("unsupported initial binding action")
	}
	return nil
}
func runInitialBinding(ctx context.Context, db *sql.DB, client *mongo.Client, pair migration.PairConfig, b initialBindingArguments, out io.Writer) error {
	var plan migration.InitialBindingPlan
	var digest string
	var err error
	if b.action == "preview" {
		plan, err = migration.PreviewInitialBinding(ctx, db, client, pair, transaction.Binding{Owner: b.owner, StoreID: b.store, WriterEpoch: b.epoch}, b.operation)
		if err != nil {
			return err
		}
		digest, err = writeInitialBindingPlan(b.plan, plan)
	} else {
		plan, err = readInitialBindingPlan(b.plan, b.reviewed)
		if err != nil {
			return err
		}
		digest = b.reviewed
		if b.action == "apply" {
			result, applyErr := migration.ApplyInitialBinding(ctx, db, client, pair, plan, b.writersExcluded)
			return errors.Join(applyErr, json.NewEncoder(out).Encode(struct {
				PlanSHA256 string                         `json:"plan_sha256"`
				Result     migration.InitialBindingResult `json:"result"`
			}{digest, result}))
		}
		err = migration.VerifyInitialBinding(ctx, db, client, pair, plan)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Action            string `json:"action"`
		PlanSHA256        string `json:"plan_sha256"`
		ReadOnly          bool   `json:"read_only"`
		Verified          bool   `json:"verified"`
		CutoverAuthorized bool   `json:"cutover_authorized"`
	}{b.action, digest, true, true, false})
}
func writeInitialBindingPlan(path string, plan migration.InitialBindingPlan) (string, error) {
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(raw)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func readInitialBindingPlan(path string, approved string) (migration.InitialBindingPlan, error) {
	var plan migration.InitialBindingPlan
	want, err := hex.DecodeString(approved)
	if err != nil || len(want) != 32 || hex.EncodeToString(want) != approved {
		return plan, errors.New("reviewed raw plan digest required")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return plan, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return plan, errors.New("private original 0600 plan required")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(raw) == 0 || len(raw) > 16384 {
		return plan, errors.New("bounded original plan required")
	}
	sum := sha256.Sum256(raw)
	if !bytes.Equal(want, sum[:]) {
		return plan, errors.New("original plan digest mismatch")
	}
	// Reject duplicate keys at every object depth before typed decoding.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err = uniqueInitialJSON(decoder); err != nil {
		return plan, err
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return plan, errors.New("one original plan object required")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&plan); err != nil {
		return plan, err
	}
	return plan, nil
}
func uniqueInitialJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			s = strings.ToLower(s)
			if !ok || seen[s] {
				return errors.New("duplicate original plan key")
			}
			seen[s] = true
			if err = uniqueInitialJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = uniqueInitialJSON(d); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid original plan JSON")
	}
	_, err = d.Token()
	return err
}
