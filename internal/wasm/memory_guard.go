package wasm

import (
	"errors"
	"os"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
)

// guardedLinearMemory is a LinearMemory backed by a large PROT_NONE virtual
// reservation whose committed prefix grows via mprotect. The base address is
// stable for the lifetime of the memory, and any out-of-bounds access up to
// the guard window faults instead of reading/writing unrelated data, which
// is what allows the compiler to omit per-access bounds checks.
type guardedLinearMemory struct {
	region    []byte // the whole reservation
	max       uint64
	committed uintptr
}

func newGuardedLinearMemory(maxBytes uint64) (*guardedLinearMemory, error) {
	region, err := platform.ReserveGuardRegion(platform.GuardRegionReserveSize)
	if err != nil {
		return nil, err
	}
	base := uintptr(unsafe.Pointer(&region[0]))
	if !platform.RegisterGuardRegion(base, base+platform.GuardRegionReserveSize) {
		_ = platform.ReleaseGuardRegion(region)
		return nil, errors.New("wazero: guard-region registry is full")
	}
	return &guardedLinearMemory{region: region, max: maxBytes}, nil
}

// Reallocate implements experimental.LinearMemory.
func (g *guardedLinearMemory) Reallocate(size uint64) []byte {
	if size > g.max || size > uint64(len(g.region)) {
		return nil
	}
	pageSize := uintptr(os.Getpagesize())
	aligned := (uintptr(size) + pageSize - 1) &^ (pageSize - 1)
	if aligned > g.committed {
		if err := platform.CommitGuardRegion(g.region, aligned); err != nil {
			return nil
		}
		g.committed = aligned
	}
	if size == 0 {
		// Keep a non-zero capacity so the returned slice's data pointer
		// (the reservation base) stays well-defined for unsafe.SliceData.
		return g.region[:0:1]
	}
	return g.region[:size:size]
}

// Free implements experimental.LinearMemory.
func (g *guardedLinearMemory) Free() {
	if g.region == nil {
		return
	}
	platform.UnregisterGuardRegion(uintptr(unsafe.Pointer(&g.region[0])))
	_ = platform.ReleaseGuardRegion(g.region)
	g.region = nil
}
