//go:build (darwin || linux || freebsd) && (amd64 || arm64)

package platform

import "syscall"

// GuardPageMemorySupported is true when guard-page-backed linear memory is
// available: a 64-bit unix-like platform where a large PROT_NONE
// reservation is cheap and faults surface as recoverable Go panics under
// runtime/debug.SetPanicOnFault.
const GuardPageMemorySupported = true

// ReserveGuardRegion reserves size bytes of inaccessible (PROT_NONE)
// address space. Nothing is committed; the reservation only consumes
// virtual address space.
func ReserveGuardRegion(size uintptr) ([]byte, error) {
	return syscall.Mmap(-1, 0, int(size), syscall.PROT_NONE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
}

// CommitGuardRegion makes the first commitBytes of the reservation
// readable and writable.
func CommitGuardRegion(region []byte, commitBytes uintptr) error {
	if commitBytes == 0 {
		return nil
	}
	return syscall.Mprotect(region[:commitBytes], syscall.PROT_READ|syscall.PROT_WRITE)
}

// ReleaseGuardRegion releases the whole reservation.
func ReleaseGuardRegion(region []byte) error {
	return syscall.Munmap(region)
}
