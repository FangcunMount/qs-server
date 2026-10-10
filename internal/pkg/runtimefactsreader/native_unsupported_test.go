//go:build !linux

package runtimefactsreader

import (
	"context"
	"testing"
)

func TestUnsupportedNativeNeverQualifies(t *testing.T) {
	if _, _, e := QuerySnapshot(context.Background(), BorrowedProcess{}); e == nil {
		t.Fatal("unsupported native query qualified")
	}
}
