//go:build !darwin && !linux
// +build !darwin,!linux

package jit

const mmapFlags = 0

// Copy the code into the executable region
// and returns the byte slice of the region.
func mmapCodeSegment(code []byte) ([]byte, error) {
	panic("unsupported GOOS")
}
