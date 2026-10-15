package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func fingerprint(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

func privatePath(path string, mustExist bool) (os.FileInfo, error) {
	if !absolutePath(path) {
		return nil, errors.New("private_path_rejected")
	}
	var leaf os.FileInfo
	for p := path; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if p == path && !mustExist && os.IsNotExist(e) {
		} else if e != nil || s.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("private_path_rejected")
		} else if p == path {
			leaf = s
		} else if !s.IsDir() || (s.Mode().Perm()&022 != 0 && s.Mode()&os.ModeSticky == 0) {
			return nil, errors.New("private_ancestor_rejected")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return leaf, nil
}

func readPrivate(path, expected string, limit int64) ([]byte, error) {
	if !nonzeroHash(expected) {
		return nil, errors.New("external_fingerprint_required")
	}
	raw, e := readOwnedPrivate(path, limit)
	if e != nil {
		return nil, e
	}
	if fingerprint(raw) != expected {
		return nil, errors.New("private_file_hash_or_release_failed")
	}
	return raw, nil
}

// Only for tool-owned recovery state in the original material's fixed private
// parent. External material and approvals must still use readPrivate with an
// independently supplied raw SHA. Filesystem ownership/identity and release
// checks are identical; trusted local state has no caller-selected hash.
func readOwnedPrivate(path string, limit int64) ([]byte, error) {
	s, e := privatePath(path, true)
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Sys().(*syscall.Stat_t).Nlink != 1 || (s.Sys().(*syscall.Stat_t).Uid != 0 && s.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid())) || s.Size() < 1 || s.Size() > limit {
		return nil, errors.New("private_file_rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("private_file_open_failed")
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(s, opened) {
		_ = f.Close()
		return nil, errors.New("private_file_changed")
	}
	raw, re := io.ReadAll(io.LimitReader(f, limit+1))
	after, se := f.Stat()
	ce := f.Close()
	current, pe := os.Lstat(path)
	if re != nil || se != nil || ce != nil || pe != nil || !os.SameFile(opened, after) || !os.SameFile(after, current) || after.Mode().Perm() != 0600 || after.Sys().(*syscall.Stat_t).Nlink != 1 || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || len(raw) > int(limit) {
		return nil, errors.New("private_file_hash_or_release_failed")
	}
	return raw, nil
}

func writeExclusive(path string, raw []byte) error {
	if len(raw) < 1 || len(raw) > maxMaterialBytes {
		return errors.New("material_size_rejected")
	}
	if s, e := privatePath(path, false); e != nil || s != nil {
		return errors.New("first_capture_exists_or_path_rejected")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errors.New("first_capture_create_failed")
	}
	n, we := f.Write(raw)
	if we == nil && n != len(raw) {
		we = io.ErrShortWrite
	}
	if we == nil {
		we = f.Sync()
	}
	ce := f.Close()
	if we != nil || ce != nil {
		return errors.New("first_capture_write_or_release_failed")
	}
	if e := syncDirectory(filepath.Dir(path)); e != nil {
		return e
	}
	_, e = readPrivate(path, fingerprint(raw), maxMaterialBytes)
	return e
}

func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return errors.New("private_directory_open_failed")
	}
	se, ce := d.Sync(), d.Close()
	if se != nil || ce != nil {
		return errors.New("private_directory_release_failed")
	}
	return nil
}
