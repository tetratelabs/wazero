package wasm

const PageSize uint64 = 65536

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	Call(f *FunctionInstance, params ...uint64) (results []uint64, err error)
	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error
}
