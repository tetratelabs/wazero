package wasm

const PageSize uint64 = 65536

// Engine is the interface implemented by interpreters.
type Engine interface {
	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	Call(f *FunctionInstance, params ...uint64) (results []uint64, err error)
	// Compile compiles down the function instance.
	Compile(f *FunctionInstance) error
	// PreCompile prepares the compilation for given function instances.
	// This is called for all the instances in a module instance
	// before Compile is called. That is necessary because
	// JIT engine needs to assign unique ids to each function instance
	// before it compiles each function. Concretely, the JIT engine
	// uses the ids at the time when emitting call instructions against the yet-compiled
	// function instances.
	PreCompile(fs []*FunctionInstance) error
}
