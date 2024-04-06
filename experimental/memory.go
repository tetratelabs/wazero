package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/ctxkey"
)

// MemoryAllocator is a memory allocation hook.
type MemoryAllocator interface {
	// Make is invoked to create a new memory, with the given specification.
	// Implementations must return a []byte min bytes in length,
	// should return a []byte with at least cap capacity,
	// and be prepared to allocate up to max bytes of memory.
	Make(min, cap, max uint64) []byte

	// Grow is invoked to grow the memory to size bytes in length.
	Grow(size uint64) []byte

	// Free is invoked to free the memory.
	Free()
}

// WithMemoryAllocator registers the given MemoryAllocator into the given
// context.Context.
func WithMemoryAllocator(ctx context.Context, allocator MemoryAllocator) context.Context {
	if allocator != nil {
		return context.WithValue(ctx, ctxkey.MemoryAllocatorKey{}, allocator)
	}
	return ctx
}
