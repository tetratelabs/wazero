// Package guardsig provides the optional, cgo-based hardware-fault handler
// that makes guard-page-backed linear memory (see
// experimental.WithGuardPageMemory) effective with the optimizing compiler.
//
// Guard-page memory lets the compiler omit per-access bounds checks: an
// out-of-bounds guest access lands in an inaccessible reserved region and
// faults. The Go runtime cannot recover a fault raised at a JIT program
// counter, so this package installs a C signal handler — via a C constructor
// that runs before the Go runtime initializes, which is the condition under
// which the Go runtime forwards synchronous signals occurring in non-Go code
// to a pre-existing handler. The handler classifies the fault (address inside
// a registered guard region, stack pointer inside a registered wasm call
// stack) and redirects the thread to a compiled exit sequence that reports
// the regular wasm out-of-bounds trap. Faults that are not guest
// out-of-bounds accesses are re-raised with the default action.
//
// Usage: blank-import this package and enable guard-page memory:
//
//	import _ "github.com/tetratelabs/wazero/experimental/guardsig"
//
//	ctx := experimental.WithGuardPageMemory(context.Background())
//	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
//	// pass ctx to InstantiateModule as well.
//
// When this package is not imported, or cgo is disabled, or the platform is
// unsupported, WithGuardPageMemory silently degrades to the regular
// bounds-checked code generation: programs behave identically either way.
//
// Importing this package makes the enclosing program a cgo program. The
// wazero module itself remains pure Go unless this package is imported.
package guardsig
