package wasm

const PageSize uint64 = 65536

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given args.
	// Returns the values from the function.
	Call(f *FunctionInstance, args ...uint64) (returns []uint64, err error)
	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error
	// PreCompile prepares the compilation for given function instances.
	// This is called for all the instances in a module instance
	// before Compile is called.
	PreCompile(fs []*FunctionInstance) error
}
