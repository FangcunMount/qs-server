//go:build linux

package compatibilityretirementfence

import (
	"os"

	"golang.org/x/sys/unix"
)

func productionSSHKeyFileOptions() (sshKeyFileOptions, error) {
	if os.Geteuid() != 0 {
		return sshKeyFileOptions{}, ErrSSHKeyFile
	}
	return sshKeyFileOptions{owner: 0, anchor: "/", metadata: sshKeyNoExtendedMetadata, exchange: func(dir int, a, b string) error { return unix.Renameat2(dir, a, dir, b, unix.RENAME_EXCHANGE) }}, nil
}

func sshKeyNoExtendedMetadata(fd int) error {
	n, e := unix.Flistxattr(fd, nil)
	if e != nil || n != 0 {
		return ErrSSHKeyFile
	}
	return nil
}
