//go:build !linux

package compatibilityretirementstop

import "context"

func openLoadedMQProcess(context.Context, actualContainer, string, string) (*loadedMQProcess, error) {
	return nil, ErrLoadedMQ
}
func verifyLoadedMQExecutable(context.Context, *loadedMQProcess, string) error { return ErrLoadedMQ }
