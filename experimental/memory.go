package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
)

// MemoryAllocator is a memory allocation hook which is invoked
// to create a new LinearMemory, with the given specification:
// min is the initial and minimum length (in bytes) of the backing []byte,
// cap a suggested initial capacity, and max the maximum length
// that will ever be requested.
type MemoryAllocator interface {
	Allocate(min, cap, max uint64) LinearMemory
}

// MemoryAllocatorFunc is a convenience for defining inlining a MemoryAllocator.
type MemoryAllocatorFunc func(min, cap, max uint64) LinearMemory

// Allocate implements MemoryAllocator.Allocate.
func (f MemoryAllocatorFunc) Allocate(min, cap, max uint64) LinearMemory {
	return f(min, cap, max)
}

// LinearMemory is an expandable []byte that backs a Wasm linear memory.
type LinearMemory interface {
	// Buffer returns the backing []byte for the linear memory.
	Buffer() []byte
	// Grow the linear memory to size bytes in length.
	// To back a shared memory, Grow can't change the address
	// of the backing []byte (only its length/capacity may change).
	Grow(size uint64) []byte
	// Free the backing memory buffer.
	Free()
}

// WithMemoryAllocator registers the given MemoryAllocator into the given
// context.Context.
func WithMemoryAllocator(ctx context.Context, allocator MemoryAllocator) context.Context {
	if allocator != nil {
		return context.WithValue(ctx, expctxkeys.MemoryAllocatorKey{}, allocator)
	}
	return ctx
}
