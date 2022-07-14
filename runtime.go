package wazero

import (
	"bytes"
	"context"
	"errors"

	"github.com/tetratelabs/wazero/api"
	experimentalapi "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
)

// Runtime allows embedding of WebAssembly modules.
//
// Ex. The below is the basic initialization of wazero's WebAssembly Runtime.
//	ctx := context.Background()
//	r := wazero.NewRuntime()
//	defer r.Close(ctx) // This closes everything this Runtime created.
//
//	module, _ := r.InstantiateModuleFromBinary(ctx, wasm)
type Runtime interface {
	// NewModuleBuilder lets you create modules out of functions defined in Go.
	//
	// Ex. Below defines and instantiates a module named "env" with one function:
	//
	//	ctx := context.Background()
	//	hello := func() {
	//		fmt.Fprintln(stdout, "hello!")
	//	}
	//	_, err := r.NewModuleBuilder("env").ExportFunction("hello", hello).Instantiate(ctx, r)
	NewModuleBuilder(moduleName string) ModuleBuilder

	// CompileModule decodes the WebAssembly binary (%.wasm) or errs if invalid.
	// Any pre-compilation done after decoding wasm is dependent on RuntimeConfig or CompileConfig.
	//
	// There are two main reasons to use CompileModule instead of InstantiateModuleFromBinary:
	//	* Improve performance when the same module is instantiated multiple times under different names
	//	* Reduce the amount of errors that can occur during InstantiateModule.
	//
	// Notes
	//
	//	* The resulting module name defaults to what was binary from the custom name section.
	//	* Any pre-compilation done after decoding the source is dependent on RuntimeConfig or CompileConfig.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(ctx context.Context, binary []byte, config CompileConfig) (CompiledModule, error)

	// InstantiateModuleFromBinary instantiates a module from the WebAssembly binary (%.wasm) or errs if invalid.
	// When the context is nil, it defaults to context.Background.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	defer r.Close(ctx) // This closes everything this Runtime created.
	//
	//	module, _ := r.InstantiateModuleFromBinary(ctx, wasm)
	//
	// Notes
	//
	//	* This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	//	source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	//	* To avoid using configuration defaults, use InstantiateModule instead.
	InstantiateModuleFromBinary(ctx context.Context, source []byte) (api.Module, error)

	// Namespace is the default namespace of this runtime, and is embedded for convenience. Most users will only use the
	// default namespace.
	//
	// Advanced use cases can use NewNamespace to redefine modules of the same name. For example, to allow different
	// modules to define their own stateful "env" module.
	Namespace

	// NewNamespace creates an empty namespace which won't conflict with any other namespace including the default.
	// This is more efficient than multiple runtimes, as namespaces share a compiler cache.
	// When the context is nil, it defaults to context.Background.
	//
	// In simplest case, a namespace won't conflict if another has a module with the same name:
	//	b := assemblyscript.NewBuilder(r)
	//	m1, _ := b.InstantiateModule(ctx, r.NewNamespace(ctx))
	//	m2, _ := b.InstantiateModule(ctx, r.NewNamespace(ctx))
	//
	// This is also useful for different modules that import the same module name (like "env"), but need different
	// configuration or state. For example, one with trace logging enabled and another disabled:
	//	b := assemblyscript.NewBuilder(r)
	//
	//	// m1 has trace logging disabled
	//	ns1 := r.NewNamespace(ctx)
	//	_ = b.InstantiateModule(ctx, ns1)
	//	m1, _ := ns1.InstantiateModule(ctx, compiled, config)
	//
	//	// m2 has trace logging enabled
	//	ns2 := r.NewNamespace(ctx)
	//	_ = b.WithTraceToStdout().InstantiateModule(ctx, ns2)
	//	m2, _ := ns2.InstantiateModule(ctx, compiled, config)
	//
	// Notes
	//
	//	* The returned namespace does not inherit any modules from the runtime default namespace.
	//	* Closing the returned namespace closes any modules in it.
	//	* Closing this runtime also closes the namespace returned from this function.
	NewNamespace(context.Context) Namespace

	// CloseWithExitCode closes all the modules that have been initialized in this Runtime with the provided exit code.
	// When the context is nil, it defaults to context.Background.
	// An error is returned if any module returns an error when closed.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	defer r.CloseWithExitCode(ctx, 2) // This closes everything this Runtime created.
	//
	//	// Everything below here can be closed, but will anyway due to above.
	//	_, _ = wasi_snapshot_preview1.InstantiateSnapshotPreview1(ctx, r)
	//	mod, _ := r.InstantiateModuleFromBinary(ctx, wasm)
	CloseWithExitCode(ctx context.Context, exitCode uint32) error

	// Closer closes all namespace and compiled code by delegating to CloseWithExitCode with an exit code of zero.
	api.Closer
}

// NewRuntime returns a runtime with a configuration assigned by NewRuntimeConfig.
func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(rConfig RuntimeConfig) Runtime {
	config := rConfig.(*runtimeConfig)
	store, ns := wasm.NewStore(config.enabledFeatures, config.newEngine(config.enabledFeatures))
	return &runtime{
		store:           store,
		ns:              &namespace{store: store, ns: ns},
		enabledFeatures: config.enabledFeatures,
		isInterpreter:   config.isInterpreter,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	store           *wasm.Store
	ns              *namespace
	enabledFeatures wasm.Features
	isInterpreter   bool
	compiledModules []*compiledModule
}

// NewNamespace implements Runtime.NewNamespace.
func (r *runtime) NewNamespace(ctx context.Context) Namespace {
	return &namespace{store: r.store, ns: r.store.NewNamespace(ctx)}
}

// Module implements Namespace.Module embedded by Runtime.
func (r *runtime) Module(moduleName string) api.Module {
	return r.ns.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(ctx context.Context, binary []byte, cConfig CompileConfig) (CompiledModule, error) {
	if binary == nil {
		return nil, errors.New("binary == nil")
	}

	config := cConfig.(*compileConfig)
	if len(binary) < 4 || !bytes.Equal(binary[0:4], binaryformat.Magic) {
		return nil, errors.New("invalid binary")
	}

	internal, err := binaryformat.DecodeModule(binary, r.enabledFeatures, config.memorySizer)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct position in the wasm binary.
		return nil, err
	}

	internal.AssignModuleID(binary)

	// Now that the module is validated, cache the function definitions.
	internal.BuildFunctionDefinitions()

	c := &compiledModule{module: internal, compiledEngine: r.store.Engine}

	if c.listeners, err = buildListeners(ctx, r, internal); err != nil {
		return nil, err
	}

	if err = r.store.Engine.CompileModule(ctx, internal); err != nil {
		return nil, err
	}

	r.compiledModules = append(r.compiledModules, c)
	return c, nil
}

func buildListeners(ctx context.Context, r *runtime, internal *wasm.Module) ([]experimentalapi.FunctionListener, error) {
	if ctx == nil {
		return nil, nil
	}
	// Test to see if internal code are using an experimental feature.
	fnlf := ctx.Value(experimentalapi.FunctionListenerFactoryKey{})
	if fnlf == nil {
		return nil, nil
	}
	if !r.isInterpreter {
		return nil, errors.New("context includes a FunctionListenerFactoryKey, which is only supported in the interpreter")
	}
	factory := fnlf.(experimentalapi.FunctionListenerFactory)
	importCount := internal.ImportFuncCount()
	listeners := make([]experimentalapi.FunctionListener, len(internal.FunctionSection))
	for i := 0; i < len(listeners); i++ {
		listeners[i] = factory.NewListener(internal.FunctionDefinitionSection[uint32(i)+importCount])
	}
	return listeners, nil
}

// InstantiateModuleFromBinary implements Runtime.InstantiateModuleFromBinary
func (r *runtime) InstantiateModuleFromBinary(ctx context.Context, binary []byte) (api.Module, error) {
	if compiled, err := r.CompileModule(ctx, binary, NewCompileConfig()); err != nil {
		return nil, err
	} else {
		compiled.(*compiledModule).closeWithModule = true
		return r.InstantiateModule(ctx, compiled, NewModuleConfig())
	}
}

// InstantiateModule implements Namespace.InstantiateModule embedded by Runtime.
func (r *runtime) InstantiateModule(
	ctx context.Context,
	compiled CompiledModule,
	mConfig ModuleConfig,
) (api.Module, error) {
	return r.ns.InstantiateModule(ctx, compiled, mConfig)
}

// Close implements api.Closer embedded in Runtime.
func (r *runtime) Close(ctx context.Context) error {
	return r.CloseWithExitCode(ctx, 0)
}

// CloseWithExitCode implements Runtime.CloseWithExitCode
func (r *runtime) CloseWithExitCode(ctx context.Context, exitCode uint32) error {
	err := r.store.CloseWithExitCode(ctx, exitCode)
	for _, c := range r.compiledModules {
		if e := c.Close(ctx); e != nil && err == nil {
			err = e
		}
	}
	return err
}
