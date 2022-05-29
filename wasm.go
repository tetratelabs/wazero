package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	experimentalapi "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/sys"
)

// Runtime allows embedding of WebAssembly modules.
//
// Ex. The below is the basic initialization of wazero's WebAssembly Runtime.
//	ctx := context.Background()
//	r := wazero.NewRuntime()
//	defer r.Close(ctx) // This closes everything this Runtime created.
//
//	module, _ := r.InstantiateModuleFromCode(ctx, source)
type Runtime interface {
	// NewModuleBuilder lets you create modules out of functions defined in Go.
	//
	// Ex. Below defines and instantiates a module named "env" with one function:
	//
	//	ctx := context.Background()
	//	hello := func() {
	//		fmt.Fprintln(stdout, "hello!")
	//	}
	//	_, err := r.NewModuleBuilder("env").ExportFunction("hello", hello).Instantiate(ctx)
	NewModuleBuilder(moduleName string) ModuleBuilder

	// Module returns exports from an instantiated module or nil if there aren't any.
	Module(moduleName string) api.Module

	// CompileModule decodes the WebAssembly text or binary source or errs if invalid.
	// When the context is nil, it defaults to context.Background.
	//
	// There are two main reasons to use CompileModule instead of InstantiateModuleFromCode:
	//	* Improve performance when the same module is instantiated multiple times under different names
	//	* Reduce the amount of errors that can occur during InstantiateModule.
	//
	// Notes:
	//	* The resulting module name defaults to what was binary from the custom name section.
	//	* Any pre-compilation done after decoding the source is dependent on RuntimeConfig or CompileConfig.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(ctx context.Context, source []byte, config CompileConfig) (CompiledModule, error)

	// InstantiateModuleFromCode instantiates a module from the WebAssembly text or binary source or errs if invalid.
	// When the context is nil, it defaults to context.Background.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	defer r.Close(ctx) // This closes everything this Runtime created.
	//
	//	module, _ := r.InstantiateModuleFromCode(ctx, source)
	//
	// Notes:
	//	* This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	//	source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	//	* To avoid using configuration defaults, use InstantiateModule instead.
	InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error)

	// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
	// When the context is nil, it defaults to context.Background.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	defer r.Close(ctx) // This closes everything this Runtime created.
	//
	//	compiled, _ := r.CompileModule(ctx, source, wazero.NewCompileConfig())
	//	module, _ := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("prod"))
	//
	// While CompiledModule is pre-validated, there are a few situations which can cause an error:
	//	* The module name is already in use.
	//	* The module has a table element initializer that resolves to an index outside the Table minimum size.
	//	* The module has a start function, and it failed to execute.
	//
	// Configuration can also define different args depending on the importing module.
	//
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	defer r.Close(ctx) // This closes everything this Runtime created.
	//
	//	_, _ := wasi.InstantiateSnapshotPreview1(r)
	//	compiled, _ := r.CompileModule(ctx, source, wazero.NewCompileConfig())
	//
	//	// Initialize base configuration:
	//	config := wazero.NewModuleConfig().WithStdout(buf)
	//
	//	// Assign different configuration on each instantiation
	//	module, _ := r.InstantiateModule(ctx, compiled, config.WithName("rotate").WithArgs("rotate", "angle=90", "dir=cw"))
	//
	// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
	InstantiateModule(ctx context.Context, compiled CompiledModule, config ModuleConfig) (api.Module, error)

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
	//	_, _ = wasi.InstantiateSnapshotPreview1(ctx, r)
	//	mod, _ := r.InstantiateModuleFromCode(ctx, source)
	CloseWithExitCode(ctx context.Context, exitCode uint32) error

	// Closer closes resources initialized by this Runtime by delegating to CloseWithExitCode with an exit code of
	// zero.
	api.Closer
}

// NewRuntime returns a runtime with a configuration assigned by NewRuntimeConfig.
func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(rConfig RuntimeConfig) Runtime {
	config, ok := rConfig.(*runtimeConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.RuntimeConfig implementation: %#v", rConfig))
	}
	return &runtime{
		store:           wasm.NewStore(config.enabledFeatures, config.newEngine(config.enabledFeatures)),
		enabledFeatures: config.enabledFeatures,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	store           *wasm.Store
	enabledFeatures wasm.Features
	compiledModules []*compiledCode
}

// Module implements Runtime.Module
func (r *runtime) Module(moduleName string) api.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(ctx context.Context, source []byte, cConfig CompileConfig) (CompiledModule, error) {
	if source == nil {
		return nil, errors.New("source == nil")
	}

	config, ok := cConfig.(*compileConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.CompileConfig implementation: %#v", cConfig))
	}

	if len(source) < 8 { // Ex. less than magic+version in binary or '(module)' in text
		return nil, errors.New("invalid source")
	}

	// Peek to see if this is a binary or text format
	var decoder wasm.DecodeModule
	if bytes.Equal(source[0:4], binary.Magic) {
		decoder = binary.DecodeModule
	} else {
		decoder = text.DecodeModule
	}

	internal, err := decoder(source, r.enabledFeatures, config.memorySizer)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct source position.
		return nil, err
	}

	// Replace imports if any configuration exists to do so.
	if importRenamer := config.importRenamer; importRenamer != nil {
		for _, i := range internal.ImportSection {
			i.Module, i.Name = importRenamer(i.Type, i.Module, i.Name)
		}
	}

	internal.AssignModuleID(source)

	if err = r.store.Engine.CompileModule(ctx, internal); err != nil {
		return nil, err
	}

	c := &compiledCode{module: internal, compiledEngine: r.store.Engine}
	r.compiledModules = append(r.compiledModules, c)
	return c, nil
}

// InstantiateModuleFromCode implements Runtime.InstantiateModuleFromCode
func (r *runtime) InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error) {
	if compiled, err := r.CompileModule(ctx, source, NewCompileConfig()); err != nil {
		return nil, err
	} else {
		// *wasm.ModuleInstance for the source cannot be tracked, so we release the cache inside this function.
		defer compiled.Close(ctx)
		return r.InstantiateModule(ctx, compiled, NewModuleConfig())
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(ctx context.Context, compiled CompiledModule, mConfig ModuleConfig) (mod api.Module, err error) {
	code, ok := compiled.(*compiledCode)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.CompiledModule implementation: %#v", compiled))
	}

	config, ok := mConfig.(*moduleConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.ModuleConfig implementation: %#v", mConfig))
	}

	var sysCtx *wasm.SysContext
	if sysCtx, err = config.toSysContext(); err != nil {
		return
	}

	name := config.name
	if name == "" && code.module.NameSection != nil && code.module.NameSection.ModuleName != "" {
		name = code.module.NameSection.ModuleName
	}

	var functionListenerFactory experimentalapi.FunctionListenerFactory
	if ctx != nil { // Test to see if internal code are using an experimental feature.
		if fnlf := ctx.Value(experimentalapi.FunctionListenerFactoryKey{}); fnlf != nil {
			functionListenerFactory = fnlf.(experimentalapi.FunctionListenerFactory)
		}
	}

	mod, err = r.store.Instantiate(ctx, code.module, name, sysCtx, functionListenerFactory)
	if err != nil {
		return
	}

	for _, fn := range config.startFunctions {
		start := mod.ExportedFunction(fn)
		if start == nil {
			continue
		}
		if _, err = start.Call(ctx); err != nil {
			if _, ok := err.(*sys.ExitError); ok {
				return
			}
			err = fmt.Errorf("module[%s] function[%s] failed: %w", name, fn, err)
			return
		}
	}
	return
}

// Close implements Runtime.Close
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
