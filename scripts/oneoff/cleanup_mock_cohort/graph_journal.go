package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// One private writer owns this journal for the action. The validated hash chain
// is read once, then each intent/terminal record is appended and fsynced. No
// quadratic re-read of a many-thousand-chunk journal is needed.
type graphJournal struct {
	file     *os.File
	latest   map[string]journalEntry
	seq      int
	previous string
}

func openGraphJournal(dir string, create bool) (*graphJournal, error) {
	j := &graphJournal{latest: map[string]journalEntry{}}
	path := filepath.Join(dir, "graph-journal.jsonl")
	flags := os.O_RDWR | syscall.O_NOFOLLOW
	if create {
		flags |= os.O_CREATE
	}
	f, e := os.OpenFile(path, flags, 0600)
	if errors.Is(e, os.ErrNotExist) && !create {
		return j, nil
	}
	if e != nil {
		return nil, e
	}
	j.file = f
	fail := func(e error) (*graphJournal, error) { _ = f.Close(); return nil, e }
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fail(e)
	}
	raw, e := privateRead(path, 128*1024*1024)
	if e != nil {
		return fail(e)
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return fail(errors.New("partial graph journal remains unknown; original bytes retained"))
	}
	entries, e := readJournal(bytes.NewReader(raw))
	if e != nil {
		return fail(e)
	}
	for _, entry := range entries {
		if !hashPattern.MatchString(entry.ApprovalSHA) || entry.Action != "apply" && entry.Action != "restore" {
			return fail(errors.New("graph journal authority changed"))
		}
		j.latest[entry.Chunk] = entry
		j.seq = entry.Seq
		j.previous = digest(encode(entry))
	}
	d, e := os.Open(dir)
	if e != nil {
		return fail(e)
	}
	e = errors.Join(d.Sync(), d.Close())
	if e != nil {
		return fail(e)
	}
	return j, nil
}
func (j *graphJournal) append(entry journalEntry) error {
	if j.file == nil {
		return errors.New("readonly missing journal cannot invent historical operation intent")
	}
	entry.Seq = j.seq + 1
	entry.Previous = j.previous
	raw := append(encode(entry), '\n')
	if _, e := j.file.Seek(0, io.SeekEnd); e != nil {
		return e
	}
	n, e := j.file.Write(raw)
	if e != nil {
		return e
	}
	if n != len(raw) {
		return io.ErrShortWrite
	}
	if e = j.file.Sync(); e != nil {
		return e
	}
	j.seq = entry.Seq
	j.previous = digest(encode(entry))
	j.latest[entry.Chunk] = entry
	return nil
}
func (j *graphJournal) close() error {
	if j == nil || j.file == nil {
		return nil
	}
	e := syscall.Flock(int(j.file.Fd()), syscall.LOCK_UN)
	ce := j.file.Close()
	j.file = nil
	return errors.Join(e, ce)
}
