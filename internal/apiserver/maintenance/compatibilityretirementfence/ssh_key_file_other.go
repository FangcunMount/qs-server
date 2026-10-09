//go:build !linux

package compatibilityretirementfence

func productionSSHKeyFileOptions() (sshKeyFileOptions, error) {
	return sshKeyFileOptions{}, ErrWindowUnavailable
}
