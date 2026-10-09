//go:build linux

package compatibilityretirementfence

import "golang.org/x/sys/unix"

func sshKeyTestExchange(dir int, a, b string) error {
	return unix.Renameat2(dir, a, dir, b, unix.RENAME_EXCHANGE)
}

func sshKeyTestMetadata(fd int) error { return sshKeyNoExtendedMetadata(fd) }
