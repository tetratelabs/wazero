package wazevo

import (
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type (
	// moduleEngine implements wasm.ModuleEngine.
	moduleEngine struct {
		// opaquePtr equals &opaque[0].
		opaquePtr *byte
		parent    *compiledModule
		module    *wasm.ModuleInstance
		opaque    moduleContextOpaque
	}

	// moduleContextOpaque is the opaque byte slice of Module instance specific contents whose size
	// is only Wasm-compile-time known, hence dynamic. Its contents are basically the pointers to the module instance,
	// specific objects as well as functions. This is sometimes called "VMContext" in other Wasm runtimes.
	moduleContextOpaque []byte
)

func (m *moduleEngine) setupOpaque(offset *wazevoapi.ModuleContextOffsetData) {
	size := offset.Size()
	if size == 0 {
		return
	}
	opaque := make([]byte, size)
	m.opaque = opaque
	m.opaquePtr = &opaque[0]
}

// NewFunction implements wasm.ModuleEngine.
func (m *moduleEngine) NewFunction(index wasm.Index) api.Function {
	if index < m.module.Source.ImportFunctionCount {
		panic("TODO: directly call a imported function.")
	}

	src := m.module.Source
	typ := src.TypeSection[src.FunctionSection[index]]
	sizeOfParamResultSlice := len(typ.Results)
	if ps := len(typ.Params); ps > sizeOfParamResultSlice {
		sizeOfParamResultSlice = ps
	}
	p := m.parent
	offset := p.functionsOffsets[index]

	ce := &callEngine{
		indexInModule:          index,
		executable:             &p.executable[offset],
		parent:                 m,
		sizeOfParamResultSlice: sizeOfParamResultSlice,
	}
	ce.init()
	return ce
}

// ResolveImportedFunction implements wasm.ModuleEngine.
func (m *moduleEngine) ResolveImportedFunction(index, indexInImportedModule wasm.Index, importedModuleEngine wasm.ModuleEngine) {
	panic("TODO")
}

// LookupFunction implements wasm.ModuleEngine.
func (m *moduleEngine) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (api.Function, error) {
	panic("TODO")
}

// FunctionInstanceReference implements wasm.ModuleEngine.
func (m *moduleEngine) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference { panic("TODO") }
