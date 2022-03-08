package internalwasm

// Engine is the interface implemented by interpreter and JIT.
// This is per-store specific instance.
type Engine interface {
	// Compile compiles down the function instances in a module,
	// and returns ModuleEngine for the module.
	Compile(importedFunctions, moduleFunctions []*FunctionInstance) (ModuleEngine, error)
}

// ModuleEngine is the interface implemented by interpreters.
type ModuleEngine interface {
	// CompiledFunctionAddress returns the absolute address of the compiled function for index.
	// The returned address is stored and used as a table element.
	CompiledFunctionAddress(index Index) uintptr

	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	// The ctx's context.Context will be the outer-most ancestor of the argument to wasm.FunctionVoidReturn, etc.
	Call(ctx *ModuleContext, f *FunctionInstance, params ...uint64) (results []uint64, err error)

	// Release releases the resources allocated by functions in this ModuleEngine.
	Release() error
}
