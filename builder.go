package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// HostModuleBuilder is a way to define host functions (in Go), so that a
// WebAssembly binary (ex. %.wasm file) can import and use them.
//
// Specifically, this implements the host side of an Application Binary
// Interface (ABI) like WASI or AssemblyScript.
//
// Ex. Below defines and instantiates a module named "env" with one function:
//
//	ctx := context.Background()
//	r := wazero.NewRuntime(ctx)
//	defer r.Close(ctx) // This closes everything this Runtime created.
//
//	hello := func() {
//		fmt.Fprintln(stdout, "hello!")
//	}
//	env, _ := r.NewHostModuleBuilder("env").
//		ExportFunction("hello", hello).
//		Instantiate(ctx, r)
//
// If the same module may be instantiated multiple times, it is more efficient
// to separate steps. Ex.
//
//	compiled, _ := r.NewHostModuleBuilder("env").
//		ExportFunction("get_random_string", getRandomString).
//		Compile(ctx)
//
//	env1, _ := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("env.1"))
//
//	env2, _ := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("env.2"))
//
// # Memory
//
// All host functions act on the importing api.Module, including any memory
// exported in its binary (%.wasm file). If you are reading or writing memory,
// it is sand-boxed Wasm memory defined by the guest.
//
// Below, `m` is the importing module, defined in Wasm. `fn` is a host function
// added via ExportFunction. This means that `x` was read from memory defined
// in Wasm, not arbitrary memory in the process.
//
//	fn := func(ctx context.Context, m api.Module, offset uint32) uint32 {
//		x, _ := m.Memory().ReadUint32Le(ctx, offset)
//		return x
//	}
//
// See ExportFunction for valid host function signatures and other details.
//
// # Notes
//
//   - HostModuleBuilder is mutable: each method returns the same instance for
//     chaining.
//   - methods do not return errors, to allow chaining. Any validation errors
//     are deferred until Compile.
//   - Insertion order is not retained. Anything defined by this builder is
//     sorted lexicographically on Compile.
type HostModuleBuilder interface {
	// Note: until golang/go#5860, we can't use example tests to embed code in interface godocs.

	// ExportFunction adds a function written in Go, which a WebAssembly module can import.
	// If a function is already exported with the same name, this overwrites it.
	//
	// # Parameters
	//
	//   - exportName - The name to export. Ex "random_get"
	//   - goFunc - The `func` to export.
	//   - names - If present, the first is the api.FunctionDefinition name.
	//	  If any follow, they must match the count of goFunc's parameters.
	//
	// Ex.
	//	// Just export the function, and use "abort" in stack traces.
	// 	builder.ExportFunction("abort", env.abort)
	//	// Ensure "~lib/builtins/abort" is used in stack traces.
	//	builder.ExportFunction("abort", env.abort, "~lib/builtins/abort")
	//	// Allow function listeners to know the param names for logging, etc.
	//	builder.ExportFunction("abort", env.abort, "~lib/builtins/abort",
	//		"message", "fileName", "lineNumber", "columnNumber")
	//
	// # Valid Signature
	//
	// Noting a context exception described later, all parameters or result
	// types must match WebAssembly 1.0 (20191205) value types. This means
	// uint32, uint64, float32 or float64. Up to one result can be returned.
	//
	// Ex. This is a valid host function:
	//
	//	addInts := func(x, y uint32) uint32 {
	//		return x + y
	//	}
	//
	// Host functions may also have an initial parameter (param[0]) of type
	// context.Context or api.Module.
	//
	// Ex. This uses a Go Context:
	//
	//	addInts := func(ctx context.Context, x, y uint32) uint32 {
	//		// add a little extra if we put some in the context!
	//		return x + y + ctx.Value(extraKey).(uint32)
	//	}
	//
	// Ex. This uses an api.Module to reads the parameters from memory. This is
	// important because there are only numeric types in Wasm. The only way to
	// share other data is via writing memory and sharing offsets.
	//
	//	addInts := func(ctx context.Context, m api.Module, offset uint32) uint32 {
	//		x, _ := m.Memory().ReadUint32Le(ctx, offset)
	//		y, _ := m.Memory().ReadUint32Le(ctx, offset + 4) // 32 bits == 4 bytes!
	//		return x + y
	//	}
	//
	// If both parameters exist, they must be in order at positions 0 and 1.
	//
	// Ex. This uses propagates context properly when calling other functions
	// exported in the api.Module:
	//	callRead := func(ctx context.Context, m api.Module, offset, byteCount uint32) uint32 {
	//		fn = m.ExportedFunction("__read")
	//		results, err := fn(ctx, offset, byteCount)
	//	--snip--
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#host-functions%E2%91%A2
	ExportFunction(exportName string, goFunc interface{}, names ...string) HostModuleBuilder

	// ExportFunctions is a convenience that calls ExportFunction for each key/value in the provided map.
	ExportFunctions(nameToGoFunc map[string]interface{}) HostModuleBuilder

	// Compile returns a CompiledModule that can instantiated in any namespace (Namespace).
	//
	// Note: Closing the Namespace has the same effect as closing the result.
	Compile(context.Context) (CompiledModule, error)

	// Instantiate is a convenience that calls Compile, then Namespace.InstantiateModule.
	// This can fail for reasons documented on Namespace.InstantiateModule.
	//
	// Ex.
	//
	//	ctx := context.Background()
	//	r := wazero.NewRuntime(ctx)
	//	defer r.Close(ctx) // This closes everything this Runtime created.
	//
	//	hello := func() {
	//		fmt.Fprintln(stdout, "hello!")
	//	}
	//	env, _ := r.NewHostModuleBuilder("env").
	//		ExportFunction("hello", hello).
	//		Instantiate(ctx, r)
	//
	// # Notes
	//
	//   - Closing the Namespace has the same effect as closing the result.
	//   - Fields in the builder are copied during instantiation: Later changes do not affect the instantiated result.
	//   - To avoid using configuration defaults, use Compile instead.
	Instantiate(context.Context, Namespace) (api.Module, error)
}

// hostModuleBuilder implements HostModuleBuilder
type hostModuleBuilder struct {
	r            *runtime
	moduleName   string
	nameToGoFunc map[string]interface{}
	funcToNames  map[string][]string
}

// NewHostModuleBuilder implements Runtime.NewHostModuleBuilder
func (r *runtime) NewHostModuleBuilder(moduleName string) HostModuleBuilder {
	return &hostModuleBuilder{
		r:            r,
		moduleName:   moduleName,
		nameToGoFunc: map[string]interface{}{},
		funcToNames:  map[string][]string{},
	}
}

// ExportFunction implements HostModuleBuilder.ExportFunction
func (b *hostModuleBuilder) ExportFunction(exportName string, goFunc interface{}, names ...string) HostModuleBuilder {
	b.nameToGoFunc[exportName] = goFunc
	if len(names) > 0 {
		b.funcToNames[exportName] = names
	}
	return b
}

// ExportFunctions implements HostModuleBuilder.ExportFunctions
func (b *hostModuleBuilder) ExportFunctions(nameToGoFunc map[string]interface{}) HostModuleBuilder {
	for k, v := range nameToGoFunc {
		b.ExportFunction(k, v)
	}
	return b
}

// Compile implements HostModuleBuilder.Compile
func (b *hostModuleBuilder) Compile(ctx context.Context) (CompiledModule, error) {
	module, err := wasm.NewHostModule(b.moduleName, b.nameToGoFunc, b.funcToNames, b.r.enabledFeatures)
	if err != nil {
		return nil, err
	} else if err = module.Validate(b.r.enabledFeatures); err != nil {
		return nil, err
	}

	c := &compiledModule{module: module, compiledEngine: b.r.store.Engine}
	if c.listeners, err = buildListeners(ctx, b.r, module); err != nil {
		return nil, err
	}

	if err = b.r.store.Engine.CompileModule(ctx, module); err != nil {
		return nil, err
	}

	return c, nil
}

// Instantiate implements HostModuleBuilder.Instantiate
func (b *hostModuleBuilder) Instantiate(ctx context.Context, ns Namespace) (api.Module, error) {
	if compiled, err := b.Compile(ctx); err != nil {
		return nil, err
	} else {
		compiled.(*compiledModule).closeWithModule = true
		return ns.InstantiateModule(ctx, compiled, NewModuleConfig())
	}
}
