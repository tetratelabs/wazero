package platform

import (
	"sort"
	"sync"
)

// GuardRegionReserveSize is the virtual reservation for one guard-page
// backed 32-bit linear memory: the whole 4 GiB wasm address space, plus
// another 4 GiB so that any u32 base + u32 static offset + access size lands
// inside the reservation, plus a 64 KiB tail so the largest access starting
// at the very end still faults inside the region.
const GuardRegionReserveSize = 1<<33 + 1<<16

// guardRegions tracks the [base, end) address ranges of live guard-page
// backed memories so that a caught hardware fault can be classified: faults
// inside a registered region are guest out-of-bounds accesses; everything
// else is a genuine runtime error and must not be masked.
var guardRegions struct {
	mu      sync.RWMutex
	regions []guardRegion // sorted by base
}

type guardRegion struct{ base, end uintptr }

// RegisterGuardRegion records a reserved region so faults inside it can be
// recognized as guest out-of-bounds accesses, both by the Go-side fault
// translation (recovered panics from Go code under SetPanicOnFault) and by
// the optional signal handler (faults from JIT code). Returns false if the
// signal handler's registry is full, in which case the region must not be
// used as a guard-page memory.
func RegisterGuardRegion(base, end uintptr) bool {
	if !guardSigRegionAdd(base, end) {
		return false
	}
	guardRegions.mu.Lock()
	defer guardRegions.mu.Unlock()
	i := sort.Search(len(guardRegions.regions), func(i int) bool {
		return guardRegions.regions[i].base >= base
	})
	guardRegions.regions = append(guardRegions.regions, guardRegion{})
	copy(guardRegions.regions[i+1:], guardRegions.regions[i:])
	guardRegions.regions[i] = guardRegion{base: base, end: end}
	return true
}

// UnregisterGuardRegion removes a previously registered region.
func UnregisterGuardRegion(base uintptr) {
	guardSigRegionDel(base)
	guardRegions.mu.Lock()
	defer guardRegions.mu.Unlock()
	for i, r := range guardRegions.regions {
		if r.base == base {
			guardRegions.regions = append(guardRegions.regions[:i], guardRegions.regions[i+1:]...)
			return
		}
	}
}

// IsGuardRegionAddr returns true if addr falls inside a registered guard
// region. Only called on the (error) fault path.
func IsGuardRegionAddr(addr uintptr) bool {
	guardRegions.mu.RLock()
	defer guardRegions.mu.RUnlock()
	i := sort.Search(len(guardRegions.regions), func(i int) bool {
		return guardRegions.regions[i].end > addr
	})
	return i < len(guardRegions.regions) && guardRegions.regions[i].base <= addr
}
