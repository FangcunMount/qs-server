package migration

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Both approval consumers use the same stable, private-file boundary.
func readPrivateRetirementAuthorization(path, expectedSHA256 string) ([]byte, error) {
	if !retirementHashRE.MatchString(expectedSHA256) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, retirementError("pristine bootstrap explicit authorization required")
	}
	for x := path; ; x = filepath.Dir(x) {
		info, e := os.Lstat(x)
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, retirementError("bootstrap private path rejected")
		}
		if x != path && !info.IsDir() {
			return nil, retirementError("bootstrap private path rejected")
		}
		if x != path && info.Mode().Perm()&022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return nil, retirementError("bootstrap ancestor writable")
		}
		if x == filepath.Dir(x) {
			break
		}
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Sys().(*syscall.Stat_t).Nlink != 1 || (info.Sys().(*syscall.Stat_t).Uid != 0 && info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid())) || info.Size() < 1 || info.Size() > 32768 {
		return nil, retirementError("bootstrap private file rejected")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, retirementError("bootstrap private file open rejected")
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0600 || opened.Sys().(*syscall.Stat_t).Nlink != 1 {
		_ = f.Close()
		return nil, retirementError("bootstrap private file changed")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 32769))
	after, statErr := f.Stat()
	closeErr := f.Close()
	current, pathErr := os.Lstat(path)
	if e != nil || statErr != nil || closeErr != nil || pathErr != nil || !os.SameFile(opened, after) || !os.SameFile(after, current) || after.Mode().Perm() != 0600 || after.Sys().(*syscall.Stat_t).Nlink != 1 || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || len(raw) > 32768 || retirementHash(raw) != expectedSHA256 {
		return nil, retirementError("bootstrap raw authorization hash mismatch")
	}
	return raw, nil
}

func claimPrivateRetirementAuthorization(path, operationID, expectedSHA256 string) error {
	f, e := os.OpenFile(path+".consumed", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return retirementError("bootstrap authorization reuse rejected")
	}
	raw, _ := json.Marshal([]string{operationID, expectedSHA256})
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	d, e := os.Open(filepath.Dir(path))
	if e != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	e = d.Sync()
	closeErr = d.Close()
	if e != nil || closeErr != nil {
		return retirementError("bootstrap consumption receipt incomplete")
	}
	return nil
}
