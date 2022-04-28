package wasm

import "context"

// Engine is a Store-scoped mechanism to compile functions declared or imported by a module.
// This is a top-level type implemented by an interpreter or JIT compiler.
type Engine interface {
	// CompileModule implements the same method as documented on wasm.Engine.
	CompileModule(ctx context.Context, module *Module) error

	// NewModuleEngine compiles down the function instances in a module, and returns ModuleEngine for the module.
	//
	// * name is the name the module was instantiated with used for error handling.
	// * module is the source module from which moduleFunctions are instantiated. This is used for caching.
	// * importedFunctions: functions this module imports, already compiled in this engine.
	// * moduleFunctions: functions declared in this module that must be compiled.
	// * table: a possibly shared table used by this module. When nil tableInit will be nil.
	// * tableInit: a mapping of TableInstance.Table index to the function index it should point to.
	//
	// Note: Input parameters must be pre-validated with wasm.Module Validate, to ensure no fields are invalid
	// due to reasons such as out-of-bounds.
	NewModuleEngine(
		name string,
		module *Module,
		importedFunctions, moduleFunctions []*FunctionInstance,
		table *TableInstance,
		tableInit map[Index]Index,
	) (ModuleEngine, error)

	// DeleteCompiledModule releases compilation caches for the given module (source).
	// Note: it is safe to call this function for a module from which module instances are instantiated even when these
	// module instances have outstanding calls.
	DeleteCompiledModule(module *Module)
}

// ModuleEngine implements function calls for a given module.
type ModuleEngine interface {
	// Name returns the name of the module this engine was compiled for.
	Name() string

	// Call invokes a function instance f with given parameters.
	Call(ctx context.Context, m *CallContext, f *FunctionInstance, params ...uint64) (results []uint64, err error)

	// CreateFuncElementInstnace creates an ElementInstance whose references are engine-specific function pointers
	// corresponding to the given `indexes`.
	CreateFuncElementInstnace(indexes []*Index) *ElementInstance
}
