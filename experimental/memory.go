package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/ctxkey"
)

type MemoryAllocator interface {
	Make(min, cap, max uint64) []byte
	Grow(uint64) []byte
	Free()
}

func WithMemoryAllocator(ctx context.Context, allocator MemoryAllocator) context.Context {
	if allocator != nil {
		return context.WithValue(ctx, ctxkey.MemoryAllocatorKey{}, allocator)
	}
	return ctx
}
