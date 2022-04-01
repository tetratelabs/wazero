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
)

// Runtime allows embedding of WebAssembly 1.0 (20191205) modules.
//
// Ex.
//	r := wazero.NewRuntime()
//	code, _ := r.CompileModule(source)
//	module, _ := r.InstantiateModule(code)
//	defer module.Close()
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
type Runtime interface {
	// NewModuleBuilder lets you create modules out of functions defined in Go.
	//
	// Ex. Below defines and instantiates a module named "env" with one function:
	//
	//	hello := func() {
	//		fmt.Fprintln(stdout, "hello!")
	//	}
	//	_, err := r.NewModuleBuilder("env").ExportFunction("hello", hello).Instantiate()
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
	// Note: The resulting module name defaults to what was binary from the custom name section.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(source []byte) (*CompiledCode, error)

	// InstantiateModuleFromCode instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or
	// errs if invalid.
	//
	// Ex.
	//	module, _ := wazero.NewRuntime().InstantiateModuleFromCode(source)
	//	defer module.Close()
	//
	// Note: This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	// source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	InstantiateModuleFromCode(source []byte) (api.Module, error)

	// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
	//
	// Ex.
	//	r := wazero.NewRuntime()
	//	code, _ := r.CompileModule(source)
	//	module, _ := r.InstantiateModule(code)
	//	defer module.Close()
	//
	// While CompiledCode is pre-validated, there are a few situations which can cause an error:
	//  * The module name is already in use.
	//  * The module has a table element initializer that resolves to an index outside the Table minimum size.
	//  * The module has a start function, and it failed to execute.
	//
	// Note: The last value of RuntimeConfig.WithContext is used for any start function.
	InstantiateModule(code *CompiledCode) (api.Module, error)

	// InstantiateModuleWithConfig is like InstantiateModule, except you can override configuration such as the module
	// name or ENV variables.
	//
	// For example, you can use this to define different args depending on the importing module.
	//
	//	r := wazero.NewRuntime()
	//	wasi, _ := r.InstantiateModule(wazero.WASISnapshotPreview1())
	//	code, _ := r.CompileModule(source)
	//
	//	// Initialize base configuration:
	//	config := wazero.NewModuleConfig().WithStdout(buf)
	//
	//	// Assign different configuration on each instantiation
	//	module, _ := r.InstantiateModuleWithConfig(code, config.WithName("rotate").WithArgs("rotate", "angle=90", "dir=cw"))
	//
	// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
	InstantiateModuleWithConfig(code *CompiledCode, config *ModuleConfig) (mod api.Module, err error)
}

func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(config *RuntimeConfig) Runtime {
	return &runtime{
		ctx:             config.ctx,
		store:           wasm.NewStore(config.newEngine(), config.enabledFeatures),
		enabledFeatures: config.enabledFeatures,
		memoryMaxPages:  config.memoryMaxPages,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	ctx             context.Context
	store           *wasm.Store
	enabledFeatures wasm.Features
	memoryMaxPages  uint32
}

// Module implements Runtime.Module
func (r *runtime) Module(moduleName string) api.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(source []byte) (*CompiledCode, error) {
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

	return &CompiledCode{module: internal}, nil
}

// InstantiateModuleFromCode implements Runtime.InstantiateModuleFromCode
func (r *runtime) InstantiateModuleFromCode(source []byte) (api.Module, error) {
	if code, err := r.CompileModule(source); err != nil {
		return nil, err
	} else {
		return r.InstantiateModule(code)
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(code *CompiledCode) (mod api.Module, err error) {
	return r.InstantiateModuleWithConfig(code, NewModuleConfig())
}

// InstantiateModuleWithConfig implements Runtime.InstantiateModuleWithConfig
func (r *runtime) InstantiateModuleWithConfig(code *CompiledCode, config *ModuleConfig) (mod api.Module, err error) {
	var sys *wasm.SysContext
	if sys, err = config.toSysContext(); err != nil {
		return
	}

	name := config.name
	if name == "" && code.module.NameSection != nil && code.module.NameSection.ModuleName != "" {
		name = code.module.NameSection.ModuleName
	}

	mod, err = r.store.Instantiate(r.ctx, code.module, name, sys)
	if err != nil {
		return
	}

	for _, fn := range config.startFunctions {
		start := mod.ExportedFunction(fn)
		if start == nil {
			continue
		}
		if _, err = start.Call(mod.WithContext(r.ctx)); err != nil {
			err = fmt.Errorf("module[%s] function[%s] failed: %w", name, fn, err)
			return
		}
	}
	return
}
