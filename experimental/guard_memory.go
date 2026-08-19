package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
)

// WithGuardPageMemory enables guard-page-backed linear memory.
//
// When enabled, linear memories are placed in a large reserved virtual
// region (covering the whole 32-bit wasm address space plus the maximum
// static offset) whose uncommitted portion is inaccessible. Out-of-bounds
// accesses are then caught by the hardware via a recoverable fault instead
// of explicit per-access bounds checks, which lets the optimizing compiler
// omit those checks entirely. Out-of-bounds accesses still trap with the
// exact same error as before. This mirrors the virtual-memory technique
// used by other WebAssembly runtimes and can substantially speed up
// memory-intensive workloads.
//
// Notes:
//   - The returned context must be passed to both wazero.NewRuntimeWithConfig
//     (it configures the compiler) and module instantiation (it configures
//     the memory allocation). Mismatches fail instantiation rather than run
//     without the safety property.
//   - Only supported with the optimizing compiler on 64-bit unix-like
//     platforms; elsewhere it is silently ignored and the regular
//     bounds-checked code is generated.
//   - Shared (threads) memories are not supported yet and fall back to
//     bounds-checked code.
//   - Address space, not physical memory, is reserved (8 GiB + guard tail
//     per memory instance).
func WithGuardPageMemory(ctx context.Context) context.Context {
	return context.WithValue(ctx, expctxkeys.GuardPageMemoryKey{}, true)
}
