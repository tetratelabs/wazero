package wasm

import "context"

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	// The ctx parameter will be the outer-most ancestor of api.HostFunctionCallContext Context.
	Call(ctx context.Context, f *FunctionInstance, params ...uint64) (results []uint64, err error)
	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error
}
