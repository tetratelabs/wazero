package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
)

// WithInterpreterLazyLowering makes the interpreter lower each function body
// on its first call, instead of lowering every function when the module is
// compiled. Pass the returned context to wazero.Runtime CompileModule (or
// Instantiate). It has no effect on the compiler engine.
//
// This reduces the memory a compiled module holds and the time to compile it
// when only part of the module is executed, at the cost of lowering on the
// first call of each function. Modules are still fully validated when
// compiled, but a lowering error is reported on the first call of the
// function rather than by CompileModule.
//
// A module compiled once is shared through the wazero.CompilationCache, so
// the context that compiles a module first decides how it is lowered.
func WithInterpreterLazyLowering(ctx context.Context) context.Context {
	return context.WithValue(ctx, expctxkeys.InterpreterLazyLowering{}, true)
}
