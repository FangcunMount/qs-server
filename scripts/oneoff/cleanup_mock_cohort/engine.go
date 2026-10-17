package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

type journalEntry struct {
	Seq         int    `json:"seq"`
	Previous    string `json:"previous"`
	Action      string `json:"action"`
	Chunk       string `json:"chunk"`
	ApprovalSHA string `json:"approval_sha256"`
	State       string `json:"state"`
}

func privateDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("absolute private archive path required")
	}
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink archive path rejected")
		}
		if _, e = os.Stat(filepath.Join(p, ".git")); e == nil {
			return errors.New("sensitive archive must stay outside Git worktrees")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	s, e := os.Stat(path)
	if e != nil {
		return e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || !s.IsDir() || s.Mode().Perm() != 0700 || int(st.Uid) != os.Geteuid() {
		return errors.New("archive must be owned private 0700 directory")
	}
	return nil
}

func privateRead(path string, max int64) ([]byte, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	s, e := f.Stat()
	if e != nil {
		return nil, e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || int(st.Uid) != os.Geteuid() || st.Nlink != 1 || s.Size() > max {
		return nil, errors.New("private owned single-link bounded file required")
	}
	raw, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(raw)) > max {
		return nil, errors.New("file read bound failed")
	}
	return raw, nil
}

func writeNew(path string, raw []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(raw); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	read, e := privateRead(path, int64(len(raw)))
	if e != nil || !bytes.Equal(read, raw) {
		return errors.New("private material readback failed")
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	e = d.Sync()
	ce = d.Close()
	if e != nil {
		return e
	}
	return ce
}

func readExact(path, want string, max int64, out any) error {
	if !hashPattern.MatchString(want) {
		return errors.New("explicit raw SHA256 required")
	}
	raw, e := privateRead(path, max)
	if e != nil {
		return e
	}
	if digest(raw) != want {
		return errors.New("input raw hash changed")
	}
	return strictDecode(raw, out)
}

func appendJournal(dir string, entry journalEntry) error {
	path := filepath.Join(dir, "journal.jsonl")
	f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer func() { _ = f.Close() }()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return e
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	s, e := f.Stat()
	if e != nil {
		return e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || s.Mode().Perm() != 0600 || int(st.Uid) != os.Geteuid() || st.Nlink != 1 {
		return errors.New("journal ownership changed")
	}
	entries, e := readJournal(f)
	if e != nil {
		return e
	}
	entry.Seq = len(entries) + 1
	if len(entries) > 0 {
		entry.Previous = digest(encode(entries[len(entries)-1]))
	}
	if _, e = f.Seek(0, io.SeekEnd); e != nil {
		return e
	}
	if _, e = f.Write(append(encode(entry), '\n')); e != nil {
		return e
	}
	return f.Sync()
}

func readJournal(r io.Reader) ([]journalEntry, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 4096), 1024*1024)
	out := []journalEntry{}
	prev := ""
	for s.Scan() {
		var e journalEntry
		if err := strictDecode(s.Bytes(), &e); err != nil {
			return nil, err
		}
		if e.Seq != len(out)+1 || e.Previous != prev || !hashPattern.MatchString(e.Chunk) {
			return nil, errors.New("journal chain invalid")
		}
		switch e.State {
		case "intent", "committed", "unknown", "reconciled_absent", "reconciled_original":
		default:
			return nil, errors.New("unknown journal state")
		}
		out = append(out, e)
		prev = digest(encode(e))
	}
	return out, s.Err()
}

func journalState(dir, chunk string) (journalEntry, bool, error) {
	raw, e := privateRead(filepath.Join(dir, "journal.jsonl"), 128*1024*1024)
	if errors.Is(e, os.ErrNotExist) {
		return journalEntry{}, false, nil
	}
	if e != nil {
		return journalEntry{}, false, e
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return journalEntry{}, false, errors.New("truncated journal is not an acknowledged phase")
	}
	entries, e := readJournal(bytes.NewReader(raw))
	if e != nil {
		return journalEntry{}, false, e
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Chunk == chunk {
			return entries[i], true, nil
		}
	}
	return journalEntry{}, false, nil
}

func plan(ctx context.Context, s dataStore, g group, p protected) (group, error) {
	if e := validateGroup(g, p); e != nil {
		return g, e
	}
	if e := s.identity(ctx, g.QS.Binding); e != nil {
		return g, e
	}
	objects := []object{}
	for _, o := range sortedObjects(g.QS.Objects) {
		if !writableLeaves[o.Store+"/"+o.Name] {
			g.QS.Exclusions = append(g.QS.Exclusions, fmt.Sprintf("%s/%s/%s: semantic reference graph pending", o.Store, o.Name, objectID(o)))
			continue
		}
		r, exists, e := s.read(ctx, o)
		if e != nil {
			return g, e
		}
		if !exists {
			return g, errors.New("explicit target is absent; no phantom planned zero")
		}
		o.Fingerprint = fingerprint(r)
		objects = append(objects, o)
	}
	g.QS.Objects = objects
	if len(objects) == 0 {
		return g, errors.New("no independently verified writable component")
	}
	return g, s.identity(ctx, g.QS.Binding)
}

func capture(ctx context.Context, s dataStore, g group, p protected, dir, groupSHA, source string) (material, error) {
	m := material{Format: 1, Kind: "mock_cohort_archive", Operation: g.Operation, GroupSHA: groupSHA, ProtectedSHA: g.ProtectedSHA, ToolSource: source, Binding: g.QS.Binding}
	if e := validateGroup(g, p); e != nil {
		return m, e
	}
	if e := privateDirectory(dir); e != nil {
		return m, e
	}
	if e := s.identity(ctx, g.QS.Binding); e != nil {
		return m, e
	}
	targets := sortedObjects(g.QS.Objects)
	guards := sortedObjects(append(append(append([]object{}, p.QS.System...), p.QS.Clack...), p.QS.Other...))
	for groupIndex, objects := range [][]object{targets, guards} {
		for i, o := range objects {
			r, exists, e := s.read(ctx, o)
			if e != nil {
				return m, e
			}
			if !exists {
				return m, errors.New("capture target/protected row missing")
			}
			if groupIndex == 0 {
				if !writableLeaves[o.Store+"/"+o.Name] {
					return m, errors.New("unimplemented semantic graph may not be captured for deletion")
				}
				if e = assertFingerprint(r, o); e != nil {
					return m, e
				}
			}
			name := fmt.Sprintf("%d-%09d.json", groupIndex, i)
			raw := encode(r)
			if len(raw) > 32*1024*1024 {
				return m, errors.New("single object archive exceeds fixed 32MiB")
			}
			if e = writeNew(filepath.Join(dir, name), raw); e != nil {
				return m, e
			}
			ref := recordRef{Object: o, File: name, SHA: digest(raw), Bytes: len(raw)}
			if groupIndex == 0 {
				m.Records = append(m.Records, ref)
			} else {
				m.Protected = append(m.Protected, ref)
			}
		}
	}
	return m, s.identity(ctx, g.QS.Binding)
}

var recordName = regexp.MustCompile(`^[01]-[0-9]{9}\.json$`)

func archived(dir string, r recordRef) (record, error) {
	var out record
	if !recordName.MatchString(r.File) {
		return out, errors.New("invalid archive member")
	}
	if r.Bytes <= 0 || r.Bytes > 32*1024*1024 {
		return out, errors.New("invalid archived object size")
	}
	e := readExact(filepath.Join(dir, r.File), r.SHA, int64(r.Bytes), &out)
	if e != nil {
		return out, e
	}
	if objectID(out.Object) != objectID(r.Object) {
		return out, errors.New("archive identity mismatch")
	}
	if out.Object.Store == "mysql" {
		seen := map[string]bool{}
		for _, c := range out.Columns {
			if !identifier.MatchString(c.Name) || seen[c.Name] || c.Type == "" || c.Null && c.Raw != nil || !c.Null && c.Raw == nil {
				return out, errors.New("invalid typed SQL archive cell")
			}
			seen[c.Name] = true
		}
		if len(seen) == 0 || len(out.BSON) != 0 {
			return out, errors.New("invalid SQL archive shape")
		}
	}
	return out, nil
}

func protect(ctx context.Context, s dataStore, m material, dir string) error {
	if e := s.identity(ctx, m.Binding); e != nil {
		return e
	}
	for _, ref := range m.Protected {
		old, e := archived(dir, ref)
		if e != nil {
			return e
		}
		current, exists, e := s.read(ctx, ref.Object)
		if e != nil {
			return e
		}
		if !exists || !sameRecord(current, old) {
			return errors.New("protected original object changed; all components stop")
		}
	}
	return nil
}

func batches(m material) [][]recordRef {
	out := [][]recordRef{}
	var batch []recordRef
	last := ""
	bytesInBatch := 0
	for _, r := range m.Records {
		limit := 1000
		if r.Object.Store == "mongo" {
			limit = 500
		}
		scope := r.Object.Store + "/" + r.Object.Name
		if len(batch) > 0 && (scope != last || len(batch) >= limit || bytesInBatch+r.Bytes > 64*1024*1024) {
			out = append(out, batch)
			batch = nil
			bytesInBatch = 0
		}
		batch = append(batch, r)
		bytesInBatch += r.Bytes
		last = scope
	}
	if len(batch) > 0 {
		out = append(out, batch)
	}
	return out
}

func checkApproval(a approval, m material, g group, archiveSHA, action, source string, now time.Time) error {
	if a.Format != 1 || a.Kind != "mock_cohort_action_approval" || a.Action != action || a.Operation != m.Operation || a.GroupSHA != m.GroupSHA || a.ProtectedSHA != m.ProtectedSHA || a.ArchiveSHA != archiveSHA || a.ToolSource != source || m.ToolSource != source || g.Operation != m.Operation || !hashPattern.MatchString(a.FreezeSHA) || !hashPattern.MatchString(a.SettlementSHA) {
		return errors.New("independent exact action approval does not bind this operation")
	}
	if !a.Issued.Before(now) || !now.Before(a.Expires) || a.Expires.Sub(a.Issued) > 30*time.Minute {
		return errors.New("approved absolute window invalid or expired")
	}
	return nil
}

func mutate(ctx context.Context, s dataStore, g group, m material, dir string, a approval, approvalSHA string, restore bool) error {
	if e := privateDirectory(dir); e != nil {
		return e
	}
	if e := protect(ctx, s, m, dir); e != nil {
		return e
	}
	if len(m.Records) != len(g.QS.Objects) || len(m.Protected) == 0 {
		return errors.New("archive is incomplete for the exact group")
	}
	for i, r := range m.Records {
		if objectID(r.Object) != objectID(g.QS.Objects[i]) {
			return errors.New("archive target order/scope changed")
		}
		if _, e := archived(dir, r); e != nil {
			return e
		}
	}
	if e := precheckTargets(ctx, s, m, dir, restore); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if !time.Now().Before(a.Expires) {
		return errors.New("approval expired before claim")
	}
	claim := filepath.Join(dir, "approval-"+approvalSHA+".consumed")
	if e := writeNew(claim, encode(a)); e != nil {
		return errors.New("approval already consumed or private claim failed")
	}
	for _, batch := range batches(m) {
		if e := ctx.Err(); e != nil {
			return e
		}
		if !time.Now().Before(a.Expires) {
			return errors.New("absolute approved window exhausted")
		}
		if e := protect(ctx, s, m, dir); e != nil {
			return e
		}
		chunk := digest(encode(batch))
		prior, found, e := journalState(dir, chunk)
		if e != nil {
			return e
		}
		if found && (prior.State == "intent" || prior.State == "unknown") {
			return errors.New("unknown outcome: run read-only verify before another approval; never replay blindly")
		}
		records := []record{}
		done := true
		for _, ref := range batch {
			r, e := archived(dir, ref)
			if e != nil {
				return e
			}
			records = append(records, r)
			current, exists, e := s.read(ctx, ref.Object)
			if e != nil {
				return e
			}
			if restore {
				if exists && !sameRecord(current, r) {
					return errors.New("restore conflicts with retained/new fact")
				}
				done = done && exists
			} else {
				if exists && !sameRecord(current, r) {
					return errors.New("delete preimage changed")
				}
				if !exists && !found {
					return errors.New("unacknowledged absence is not authorization to skip")
				}
				done = done && !exists
			}
		}
		if done {
			continue
		}
		action := "apply"
		if restore {
			action = "restore"
		}
		entry := journalEntry{Action: action, Chunk: chunk, ApprovalSHA: approvalSHA, State: "intent"}
		if e = appendJournal(dir, entry); e != nil {
			return e
		}
		e = s.mutate(ctx, records, restore)
		if e != nil {
			entry.State = "unknown"
			je := appendJournal(dir, entry)
			return errors.Join(errors.New("write/commit outcome unproven; preserved journal requires read-only reconciliation"), e, je)
		}
		entry.State = "committed"
		if e = appendJournal(dir, entry); e != nil {
			return errors.Join(errors.New("write acknowledged but durable terminal journal unproven; verify, do not resend"), e)
		}
	}
	return protect(ctx, s, m, dir)
}

func precheckTargets(ctx context.Context, s dataStore, m material, dir string, restore bool) error {
	for _, batch := range batches(m) {
		prior, found, e := journalState(dir, digest(encode(batch)))
		if e != nil {
			return e
		}
		if found && (prior.State == "intent" || prior.State == "unknown") {
			return errors.New("unknown outcome requires read-only verify before claim")
		}
		for _, ref := range batch {
			if !writableLeaves[ref.Object.Store+"/"+ref.Object.Name] {
				return errors.New("semantic graph pending for this component")
			}
			old, e := archived(dir, ref)
			if e != nil {
				return e
			}
			current, exists, e := s.read(ctx, ref.Object)
			if e != nil {
				return e
			}
			if !bytes.Equal(old.Definition, current.Definition) {
				return errors.New("original schema/namespace changed")
			}
			if exists && !sameRecord(current, old) {
				return errors.New("current row conflicts with exact preimage")
			}
			if !restore && !exists && !found {
				return errors.New("absence lacks a prior acknowledged or reconciled chunk")
			}
		}
	}
	return nil
}

func verify(ctx context.Context, s dataStore, m material, dir string, restore bool) error {
	if e := protect(ctx, s, m, dir); e != nil {
		return e
	}
	for _, batch := range batches(m) {
		chunk := digest(encode(batch))
		allAbsent, allOriginal := true, true
		for _, ref := range batch {
			old, e := archived(dir, ref)
			if e != nil {
				return e
			}
			current, exists, e := s.read(ctx, ref.Object)
			if e != nil {
				return e
			}
			if !bytes.Equal(current.Definition, old.Definition) {
				return errors.New("target schema/namespace changed")
			}
			allAbsent = allAbsent && !exists
			allOriginal = allOriginal && exists && sameRecord(current, old)
		}
		prior, found, e := journalState(dir, chunk)
		if e != nil {
			return e
		}
		if found && (prior.State == "intent" || prior.State == "unknown") {
			if !allAbsent && !allOriginal {
				return errors.New("mixed/drifted unknown chunk remains blocked")
			}
			prior.State = "reconciled_absent"
			if allOriginal {
				prior.State = "reconciled_original"
			}
			if e = appendJournal(dir, prior); e != nil {
				return e
			}
		}
		if restore && !allOriginal || !restore && !allAbsent {
			return errors.New("target state not fully verified")
		}
	}
	return protect(ctx, s, m, dir)
}
