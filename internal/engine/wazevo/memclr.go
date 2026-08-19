package wazevo

import (
	"reflect"
	"unsafe"
)

// memclrNoHeapPointers is the Go runtime's optimized memory-clearing
// routine (wide SIMD stores, and non-temporal stores for very large sizes).
// Like memmove above, it is safe to call directly from generated code: it is
// nosplit, allocation-free, and touches no pointers the GC cares about.
// Used as a fast path for memory.fill with a zero value.
//
//go:linkname memclrNoHeapPointers runtime.memclrNoHeapPointers
func memclrNoHeapPointers(_ unsafe.Pointer, _ uintptr)

var memclrPtr = reflect.ValueOf(memclrNoHeapPointers).Pointer()
