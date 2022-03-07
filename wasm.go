package wazero

import (
	"bytes"
	"errors"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasm"
)

// Runtime allows embedding of WebAssembly 1.0 (20191205) modules.
//
// Ex.
//	r := wazero.NewRuntime()
//	decoded, _ := r.DecodeModule(source)
//	module, _ := r.NewModule(decoded)
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
type Runtime interface {
	// Module returns exports from an instantiated module or nil if there aren't any.
	Module(moduleName string) wasm.Module

	// DecodeModule decodes the WebAssembly 1.0 (20191205) text or binary source or errs if invalid.
	//
	// Note: the name defaults to what was decoded from the custom name section.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	DecodeModule(source []byte) (*DecodedModule, error)

	// NewModuleFromSource instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or errs if
	// invalid.
	//
	// Ex.
	//	module, _ := wazero.NewRuntime().NewModuleFromSource(source)
	//
	// Note: This is a convenience utility that chains DecodeModule with NewModule. To instantiate the same source
	// multiple times, use DecodeModule as NewModule avoids redundant decoding and/or compilation.
	NewModuleFromSource(source []byte) (wasm.Module, error)

	// NewModule instantiates the module namespace or errs if the configuration was invalid.
	//
	// Ex.
	//	r := wazero.NewRuntime()
	//	decoded, _ := r.DecodeModule(source)
	//	module, _ := r.NewModule(decoded)
	//
	// Note: The last value of RuntimeConfig.WithContext is used for any WebAssembly 1.0 (20191205) Start ExportedFunction.
	NewModule(module *DecodedModule) (wasm.Module, error)

	// NewHostModuleFromConfig instantiates the module namespace from the host or errs if the configuration was invalid.
	//
	// Ex.
	//	module, _ := wazero.NewRuntime().NewHostModuleFromConfig(wazero.WASISnapshotPreview1())
	NewHostModuleFromConfig(hostModule *HostModuleConfig) (wasm.Module, error)

	// TODO: RemoveModule
}

func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(config *RuntimeConfig) Runtime {
	return &runtime{
		store:           internalwasm.NewStore(config.ctx, config.engine, config.enabledFeatures),
		enabledFeatures: config.enabledFeatures,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	store           *internalwasm.Store
	enabledFeatures internalwasm.Features
}

// Module implements wasm.Store Module
func (r *runtime) Module(moduleName string) wasm.Module {
	return r.store.Module(moduleName)
}

// DecodeModule implements Runtime.DecodeModule
func (r *runtime) DecodeModule(source []byte) (*DecodedModule, error) {
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

	internal, err := decoder(source, r.enabledFeatures)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct source position.
		return nil, err
	}

	result := &DecodedModule{module: internal}
	if internal.NameSection != nil {
		result.name = internal.NameSection.ModuleName
	}

	return result, nil
}

// NewModuleFromSource implements Runtime.NewModuleFromSource
func (r *runtime) NewModuleFromSource(source []byte) (wasm.Module, error) {
	if decoded, err := r.DecodeModule(source); err != nil {
		return nil, err
	} else {
		return r.NewModule(decoded)
	}
}

// NewModule implements Runtime.NewModule
func (r *runtime) NewModule(module *DecodedModule) (wasm.Module, error) {
	return r.store.Instantiate(module.module, module.name)
}

// NewHostModuleFromConfig implements Runtime.NewHostModuleFromConfig
func (r *runtime) NewHostModuleFromConfig(hostModule *HostModuleConfig) (wasm.Module, error) {
	if m, err := internalwasm.NewHostModule(hostModule.Name, hostModule.Functions); err != nil {
		return nil, err
	} else {
		return r.store.Instantiate(m, hostModule.Name)
	}
}
