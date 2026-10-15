package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

type captureParentIdentity struct {
	info      os.FileInfo
	owner     uint32
	directory *os.File
}

func captureParent(path string) (captureParentIdentity, error) {
	directory, e := privateParent(path)
	if e != nil {
		return captureParentIdentity{}, e
	}
	info, e := directory.Stat()
	if e != nil || !captureDirectoryAllowed(info) {
		_ = directory.Close()
		return captureParentIdentity{}, errors.New("capture_parent_changed")
	}
	return captureParentIdentity{info: info, owner: info.Sys().(*syscall.Stat_t).Uid, directory: directory}, nil
}
func captureDirectoryAllowed(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Getuid())
}

func (parent *captureParentIdentity) close() error {
	if parent == nil || parent.directory == nil {
		return nil
	}
	directory := parent.directory
	parent.directory = nil
	if directory.Close() != nil {
		return errors.New("private_directory_release_unproven")
	}
	return nil
}

func checkCaptureParent(path string, original *captureParentIdentity) error {
	current, e := privateParent(path)
	if e != nil {
		return e
	}
	info, se := current.Stat()
	ce := current.Close()
	if se != nil || ce != nil || original == nil || original.info == nil || original.directory == nil || !captureDirectoryAllowed(info) {
		return errors.New("capture_parent_changed")
	}
	held, e := original.directory.Stat()
	if e != nil || !captureDirectoryAllowed(held) || !os.SameFile(original.info, held) || !os.SameFile(held, info) || original.owner != held.Sys().(*syscall.Stat_t).Uid || original.owner != info.Sys().(*syscall.Stat_t).Uid {
		return errors.New("capture_parent_changed")
	}
	return nil
}

// Capture alone holds its pre-ENV parent through creation, readback and fsync.
// The pre-existing private material writer is unchanged by this capture path.
func writeCaptureExclusive(parent *captureParentIdentity, path string, raw []byte) error {
	if len(raw) < 1 || len(raw) > maxMaterialBytes {
		return errors.New("material_encoding_limit")
	}
	if parent == nil || parent.directory == nil || !absolutePath(path) {
		return errors.New("capture_parent_changed")
	}
	fd, e := unix.Openat(int(parent.directory.Fd()), filepath.Base(path), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return errors.New("new_private_capture_required")
	}
	f := os.NewFile(uintptr(fd), path)
	n, we := f.Write(raw)
	if we == nil && n != len(raw) {
		we = io.ErrShortWrite
	}
	if we == nil {
		we = f.Sync()
	}
	created, se := f.Stat()
	ce := f.Close()
	if we != nil || se != nil || ce != nil || parent.directory.Sync() != nil {
		return errors.New("capture_persistence_unproven")
	}
	return readCaptureAt(parent.directory, filepath.Base(path), created, raw)
}

func readCaptureAt(parent *os.File, name string, created os.FileInfo, expected []byte) error {
	fd, e := unix.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return errors.New("private_material_fingerprint_or_release_rejected")
	}
	f := os.NewFile(uintptr(fd), name)
	opened, se := f.Stat()
	if se != nil || !os.SameFile(created, opened) || !captureFileAllowed(opened, len(expected)) {
		_ = f.Close()
		return errors.New("private_material_fingerprint_or_release_rejected")
	}
	raw, re := io.ReadAll(io.LimitReader(f, int64(len(expected))+1))
	after, se := f.Stat()
	ce := f.Close()
	if re != nil || se != nil || ce != nil || !os.SameFile(opened, after) || !captureFileAllowed(after, len(expected)) || !opened.ModTime().Equal(after.ModTime()) || !bytes.Equal(raw, expected) {
		return errors.New("private_material_fingerprint_or_release_rejected")
	}
	// Check that the name in the held directory still identifies the file read.
	fd, e = unix.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return errors.New("private_material_fingerprint_or_release_rejected")
	}
	current := os.NewFile(uintptr(fd), name)
	info, se := current.Stat()
	ce = current.Close()
	if se != nil || ce != nil || !os.SameFile(after, info) || !captureFileAllowed(info, len(expected)) {
		return errors.New("private_material_fingerprint_or_release_rejected")
	}
	return nil
}

func captureFileAllowed(info os.FileInfo, size int) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != int64(size) {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1 && stat.Uid == uint32(os.Getuid())
}

func finishCapture(ctx context.Context, r *receipt, path string, originalParent *captureParentIdentity, raw []byte, write func(*captureParentIdentity, string, []byte) error) error {
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := checkCaptureParent(path, originalParent); e != nil {
		return e
	}
	if e := write(originalParent, path, raw); e != nil {
		return e
	}
	if e := checkCaptureParent(path, originalParent); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := originalParent.close(); e != nil {
		r.ReleaseError = e.Error()
		return e
	}
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	r.MaterialHash = fingerprint(raw)
	return nil
}
