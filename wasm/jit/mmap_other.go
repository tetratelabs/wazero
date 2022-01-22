//go:build !darwin && !linux
// +build !darwin,!linux

package jit

func mmapCodeSegment(code []byte) ([]byte, error) {
	panic("unsupported GOOS")
}
