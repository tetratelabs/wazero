package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasm"
)

// Runtime allows embedding of WebAssembly 1.0 (20191205) modules.
//
// Ex.
//	r := wazero.NewRuntime()
//	decoded, _ := r.CompileModule(source)
//	module, _ := r.InstantiateModule(decoded)
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
	Module(moduleName string) wasm.Module

	// CompileModule decodes the WebAssembly 1.0 (20191205) text or binary source or errs if invalid.
	// Any pre-compilation done after decoding the source is dependent on the RuntimeConfig.
	//
	// There are two main reasons to use CompileModule instead of InstantiateModuleFromSource:
	//  * Improve performance when the same module is instantiated multiple times under different names
	//  * Reduce the amount of errors that can occur during InstantiateModule.
	//
	// Note: The resulting module name defaults to what was decoded from the custom name section.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(source []byte) (*Module, error)

	// InstantiateModuleFromSource instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or
	// errs if invalid.
	//
	// Ex.
	//	module, _ := wazero.NewRuntime().InstantiateModuleFromSource(source)
	//	defer module.Close()
	//
	// Note: This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	// source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	InstantiateModuleFromSource(source []byte) (wasm.Module, error)

	// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
	//
	// Ex.
	//	r := wazero.NewRuntime()
	//	decoded, _ := r.CompileModule(source)
	//	module, _ := r.InstantiateModule(decoded)
	//	defer module.Close()
	//
	// While a Module is pre-validated, there are a few situations which can cause an error:
	//  * The Module name is already in use.
	//  * The Module has a table element initializer that resolves to an index outside the Table minimum size.
	//  * The Module has a start function, and it failed to execute.
	//
	// Note: The last value of RuntimeConfig.WithContext is used for any start function.
	InstantiateModule(module *Module) (wasm.Module, error)
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
func (r *runtime) Module(moduleName string) wasm.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(source []byte) (*Module, error) {
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

	result := &Module{module: internal}
	if internal.NameSection != nil {
		result.name = internal.NameSection.ModuleName
	}

	return result, nil
}

// InstantiateModuleFromSource implements Runtime.InstantiateModuleFromSource
func (r *runtime) InstantiateModuleFromSource(source []byte) (wasm.Module, error) {
	if decoded, err := r.CompileModule(source); err != nil {
		return nil, err
	} else {
		return r.InstantiateModule(decoded)
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(module *Module) (wasm.Module, error) {
	return r.store.Instantiate(r.ctx, module.module, module.name, internalwasm.DefaultSysContext())
}
