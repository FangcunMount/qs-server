//go:build darwin

package compatibilityretirementfence

import "golang.org/x/sys/unix"

// Real private APFS exchange under the actual local UID, not production/root.
func sshKeyTestExchange(dir int, a, b string) error {
	return unix.RenameatxNp(dir, a, dir, b, unix.RENAME_SWAP)
}

// macOS attaches com.apple.provenance to newly-created private fixtures. It
// persists even after a successful native removal call. The private Darwin
// adapter checks actual FD names and permits only that OS attribute. Production
// Linux strictly requires no attributes and public Darwin is unavailable.
func sshKeyTestMetadata(fd int) error {
	data := make([]byte, 4096)
	n, e := unix.Flistxattr(fd, data)
	if e != nil || n < 0 || n > len(data) {
		return ErrSSHKeyFile
	}
	if n == 0 || string(data[:n]) == "com.apple.provenance\x00" {
		return nil
	}
	return ErrSSHKeyFile
}
