package internalwasm

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	// The ctx's context.Context will be the outer-most ancestor of the argument to wasm.FunctionVoidReturn, etc.
	Call(ctx *HostFunctionCallContext, f *FunctionInstance, params ...uint64) (results []uint64, err error)

	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error
}
