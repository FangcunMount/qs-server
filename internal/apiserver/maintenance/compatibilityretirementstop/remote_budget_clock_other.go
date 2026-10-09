//go:build !linux

package compatibilityretirementstop

func remoteBootClock() (string, int64, error) { return "", 0, ErrRemoteBudget }
