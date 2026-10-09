//go:build !linux

package runtimefacts

import (
	"errors"
	"net"
)

func nativeIdentity() (ProcessIdentity, error) {
	return ProcessIdentity{}, errors.New("runtime facts native Linux identity unavailable")
}
func createNativeEndpoint(string) (nativeEndpoint, error) {
	return nil, errors.New("runtime facts native Linux private channel unavailable")
}
func verifyNativePeer(*net.UnixConn, int) error {
	return errors.New("runtime facts native Linux peer credentials unavailable")
}
