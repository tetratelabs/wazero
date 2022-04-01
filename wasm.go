package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
)

// Runtime allows embedding of WebAssembly 1.0 (20191205) modules.
//
// Ex.
//	r := wazero.NewRuntime()
//	binary, _ := r.CompileModule(source)
//	module, _ := r.InstantiateModule(binary)
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
	CompileModule(source []byte) (*Binary, error)

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
	//	binary, _ := r.CompileModule(source)
	//	module, _ := r.InstantiateModule(binary)
	//	defer module.Close()
	//
	// While a Binary is pre-validated, there are a few situations which can cause an error:
	//  * The Binary name is already in use.
	//  * The Binary has a table element initializer that resolves to an index outside the Table minimum size.
	//  * The Binary has a start function, and it failed to execute.
	//
	// Note: The last value of RuntimeConfig.WithContext is used for any start function.
	InstantiateModule(binary *Binary) (api.Module, error)

	// InstantiateModuleWithConfig is like InstantiateModule, except you can override configuration such as the module
	// name or ENV variables.
	//
	// For example, you can use this to define different args depending on the importing module.
	//
	//	r := wazero.NewRuntime()
	//	wasi, _ := r.InstantiateModule(wazero.WASISnapshotPreview1())
	//	binary, _ := r.CompileModule(source)
	//
	//	// Initialize base configuration:
	//	config := wazero.NewModuleConfig().WithStdout(buf)
	//
	//	// Assign different configuration on each instantiation
	//	module, _ := r.InstantiateModuleWithConfig(binary, config.WithName("rotate").WithArgs("rotate", "angle=90", "dir=cw"))
	//
	// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
	InstantiateModuleWithConfig(binary *Binary, config *ModuleConfig) (mod api.Module, err error)
}

func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(config *RuntimeConfig) Runtime {
	return &runtime{
		ctx:             config.ctx,
		store:           internalwasm.NewStore(config.newEngine(), config.enabledFeatures),
		enabledFeatures: config.enabledFeatures,
		memoryMaxPages:  config.memoryMaxPages,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	ctx             context.Context
	store           *internalwasm.Store
	enabledFeatures internalwasm.Features
	memoryMaxPages  uint32
}

// Module implements Runtime.Module
func (r *runtime) Module(moduleName string) api.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(source []byte) (*Binary, error) {
	if source == nil {
		return nil, errors.New("source == nil")
	}

	if len(source) < 8 { // Ex. less than magic+version in binary or '(module)' in text
		return nil, errors.New("invalid source")
	}

	// Peek to see if this is a binary or text format
	var decoder internalwasm.DecodeModule
	if bytes.Equal(source[0:4], binary.Magic) {
		decoder = binary.DecodeModule
	} else {
		decoder = text.DecodeModule
	}

	if r.memoryMaxPages > internalwasm.MemoryMaxPages {
		return nil, fmt.Errorf("memoryMaxPages %d (%s) > specification max %d (%s)",
			r.memoryMaxPages, internalwasm.PagesToUnitOfBytes(r.memoryMaxPages),
			internalwasm.MemoryMaxPages, internalwasm.PagesToUnitOfBytes(internalwasm.MemoryMaxPages))
	}

	internal, err := decoder(source, r.enabledFeatures, r.memoryMaxPages)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct source position.
		return nil, err
	}

	return &Binary{module: internal}, nil
}

// InstantiateModuleFromCode implements Runtime.InstantiateModuleFromCode
func (r *runtime) InstantiateModuleFromCode(source []byte) (api.Module, error) {
	if binary, err := r.CompileModule(source); err != nil {
		return nil, err
	} else {
		return r.InstantiateModule(binary)
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(binary *Binary) (mod api.Module, err error) {
	return r.InstantiateModuleWithConfig(binary, NewModuleConfig())
}

// InstantiateModuleWithConfig implements Runtime.InstantiateModuleWithConfig
func (r *runtime) InstantiateModuleWithConfig(binary *Binary, config *ModuleConfig) (mod api.Module, err error) {
	var sys *internalwasm.SysContext
	if sys, err = config.toSysContext(); err != nil {
		return
	}

	name := config.name
	if name == "" && binary.module.NameSection != nil && binary.module.NameSection.ModuleName != "" {
		name = binary.module.NameSection.ModuleName
	}

	mod, err = r.store.Instantiate(r.ctx, binary.module, name, sys)
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
