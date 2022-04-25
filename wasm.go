package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/sys"
)

// Runtime allows embedding of WebAssembly 1.0 (20191205) modules.
//
// Ex.
//	ctx := context.Background()
//	r := wazero.NewRuntime()
//	compiled, _ := r.CompileModule(ctx, source)
//	module, _ := r.InstantiateModule(ctx, compiled)
//	defer module.Close()
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
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

	// CompileModule decodes the WebAssembly 1.0 (20191205) text or binary source or errs if invalid.
	// Any pre-compilation done after decoding the source is dependent on the RuntimeConfig.
	//
	// There are two main reasons to use CompileModule instead of InstantiateModuleFromCode:
	//  * Improve performance when the same module is instantiated multiple times under different names
	//  * Reduce the amount of errors that can occur during InstantiateModule.
	//
	// Note: When the context is nil, it defaults to context.Background.
	// Note: The resulting module name defaults to what was binary from the custom name section.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(ctx context.Context, source []byte) (*CompiledCode, error)

	// InstantiateModuleFromCode instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or
	// errs if invalid.
	//
	// Ex.
	//	ctx := context.Background()
	//	module, _ := wazero.NewRuntime().InstantiateModuleFromCode(ctx, source)
	//	defer module.Close()
	//
	// Note: When the context is nil, it defaults to context.Background.
	// Note: This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	// source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error)

	// InstantiateModuleFromCodeWithConfig is a convenience function that chains CompileModule to
	// InstantiateModuleWithConfig.
	//
	// Ex. To only change the module name:
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	wasm, _ := r.InstantiateModuleFromCodeWithConfig(ctx, source, wazero.NewModuleConfig().
	//		WithName("wasm")
	//	)
	//	defer wasm.Close()
	//
	// Note: When the context is nil, it defaults to context.Background.
	InstantiateModuleFromCodeWithConfig(ctx context.Context, source []byte, config *ModuleConfig) (api.Module, error)

	// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	compiled, _ := r.CompileModule(ctx, source)
	//	defer compiled.Close()
	//	module, _ := r.InstantiateModule(ctx, compiled)
	//	defer module.Close()
	//
	// While CompiledCode is pre-validated, there are a few situations which can cause an error:
	//  * The module name is already in use.
	//  * The module has a table element initializer that resolves to an index outside the Table minimum size.
	//  * The module has a start function, and it failed to execute.
	//
	// Note: When the context is nil, it defaults to context.Background.
	InstantiateModule(ctx context.Context, compiled *CompiledCode) (api.Module, error)

	// InstantiateModuleWithConfig is like InstantiateModule, except you can override configuration such as the module
	// name or ENV variables.
	//
	// For example, you can use this to define different args depending on the importing module.
	//
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	wasi, _ := wasi.InstantiateSnapshotPreview1(r)
	//	compiled, _ := r.CompileModule(ctx, source)
	//
	//	// Initialize base configuration:
	//	config := wazero.NewModuleConfig().WithStdout(buf)
	//
	//	// Assign different configuration on each instantiation
	//	module, _ := r.InstantiateModuleWithConfig(ctx, compiled, config.WithName("rotate").WithArgs("rotate", "angle=90", "dir=cw"))
	//
	// Note: When the context is nil, it defaults to context.Background.
	// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
	InstantiateModuleWithConfig(ctx context.Context, compiled *CompiledCode, config *ModuleConfig) (mod api.Module, err error)
}

func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(config *RuntimeConfig) Runtime {
	return &runtime{
		store:           wasm.NewStore(config.enabledFeatures, config.newEngine(config.enabledFeatures)),
		enabledFeatures: config.enabledFeatures,
		memoryMaxPages:  config.memoryMaxPages,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	enabledFeatures         wasm.Features
	store                   *wasm.Store
	memoryMaxPages          uint32
	functionListenerFactory wasm.FunctionListenerFactory
}

// Module implements Runtime.Module
func (r *runtime) Module(moduleName string) api.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(ctx context.Context, source []byte) (*CompiledCode, error) {
	if source == nil {
		return nil, errors.New("source == nil")
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

	if r.memoryMaxPages > wasm.MemoryMaxPages {
		return nil, fmt.Errorf("memoryMaxPages %d (%s) > specification max %d (%s)",
			r.memoryMaxPages, wasm.PagesToUnitOfBytes(r.memoryMaxPages),
			wasm.MemoryMaxPages, wasm.PagesToUnitOfBytes(wasm.MemoryMaxPages))
	}

	internal, err := decoder(source, r.enabledFeatures, r.memoryMaxPages)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct source position.
		return nil, err
	}

	internal.AssignModuleID(source)

	if err = r.store.Engine.CompileModule(ctx, internal); err != nil {
		return nil, err
	}

	return &CompiledCode{module: internal, compiledEngine: r.store.Engine}, nil
}

// InstantiateModuleFromCode implements Runtime.InstantiateModuleFromCode
func (r *runtime) InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error) {
	if compiled, err := r.CompileModule(ctx, source); err != nil {
		return nil, err
	} else {
		// *wasm.ModuleInstance for the source cannot be tracked, so we release the cache inside this function.
		defer compiled.Close(ctx)
		return r.InstantiateModule(ctx, compiled)
	}
}

// InstantiateModuleFromCodeWithConfig implements Runtime.InstantiateModuleFromCodeWithConfig
func (r *runtime) InstantiateModuleFromCodeWithConfig(ctx context.Context, source []byte, config *ModuleConfig) (api.Module, error) {
	if compiled, err := r.CompileModule(ctx, source); err != nil {
		return nil, err
	} else {
		// *wasm.ModuleInstance for the source cannot be tracked, so we release the cache inside this function.
		defer compiled.Close(ctx)
		return r.InstantiateModuleWithConfig(ctx, compiled, config)
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(ctx context.Context, compiled *CompiledCode) (mod api.Module, err error) {
	return r.InstantiateModuleWithConfig(ctx, compiled, NewModuleConfig())
}

// InstantiateModuleWithConfig implements Runtime.InstantiateModuleWithConfig
func (r *runtime) InstantiateModuleWithConfig(ctx context.Context, compiled *CompiledCode, config *ModuleConfig) (mod api.Module, err error) {
	var sysCtx *wasm.SysContext
	if sysCtx, err = config.toSysContext(); err != nil {
		return
	}

	name := config.name
	if name == "" && compiled.module.NameSection != nil && compiled.module.NameSection.ModuleName != "" {
		name = compiled.module.NameSection.ModuleName
	}

	module := config.replaceImports(compiled.module)

	mod, err = r.store.Instantiate(ctx, module, name, sysCtx, r.functionListenerFactory)
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

func (r *runtime) WithFunctionListenerFactory(factory wasm.FunctionListenerFactory) {
	r.functionListenerFactory = factory
}
