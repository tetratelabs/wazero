package internalwasm

// Engine is a Store-scoped mechanism to compile functions declared or imported by a module.
// This is a top-level type implemented by an interpreter or JIT compiler.
type Engine interface {
	// NewModuleEngine compiles down the function instances in a module, and returns ModuleEngine for the module.
	//
	// * importedFunctions: functions this module imports, already compiled in this engine.
	// * moduleFunctions: functions declared in this module that must be compiled.
	//
	// Note: Input parameters must be pre-validated with internalwasm.Module Validate, to ensure no fields are invalid
	// due to reasons such as out-of-bounds.
	NewModuleEngine(importedFunctions, moduleFunctions []*FunctionInstance) (ModuleEngine, error)
}

// ModuleEngine implements function calls for a given module.
type ModuleEngine interface {
	// FunctionAddress returns the absolute address of the compiled function for index.
	// The returned address is stored and used as a TableInstance.Table element.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr
	FunctionAddress(index Index) uintptr

	// Call invokes a function instance f with given parameters.
	// Returns the results from the function.
	// The ctx's context.Context will be the outer-most ancestor of the argument to wasm.Function.
	Call(ctx *ModuleContext, f *FunctionInstance, params ...uint64) (results []uint64, err error)

	// Close releases the resources allocated by functions in this ModuleEngine.
	Close()
}
