package platform

const nfdbits = 0x40

// FdSet is a set of FDs to be used with Select.
//
// This needs to be declared explicitly instead of re-exporting syscall.FdSet
// because it is missing from Go 1.18.
type FdSet struct {
	Bits [16]int64
}
