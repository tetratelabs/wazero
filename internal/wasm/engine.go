package internalwasm

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	// The ctx's context.Context will be the outer-most ancestor of the argument to wasm.FunctionVoidReturn, etc.
	Call(ctx *ModuleContext, f *FunctionInstance, params ...uint64) (results []uint64, err error)

	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error

	// Release releases the resources allocated by a function instance.
	// Note: this is only called after ensuring that no existing function will call the release target.
	// Therefore, it is safe to reuse the resource, for example reallocate the new compiled function
	// for the same FunctionIndex.
	Release(f *FunctionInstance) error
}
