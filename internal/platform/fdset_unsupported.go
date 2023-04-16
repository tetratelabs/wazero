//go:build !darwin && !unix

package platform

const nfdbits = 0x40

// FdSet mocks syscall.FdSet on systems that do not support it.
type FdSet struct {
	Bits [64]int32
}
