//go:build !linux

package runtimefactsreader

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
)

func QuerySnapshot(context.Context, BorrowedProcess) (runtimefacts.Snapshot, NativeIdentity, error) {
	return runtimefacts.Snapshot{}, NativeIdentity{}, errors.New("runtime facts reader requires original Linux root process owner")
}
