package wazevo

import (
	"encoding/binary"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
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
	//
	// Internally, the buffer is structured as follows:
	//
	// 	type moduleContextOpaque struct {
	// 	    localMemoryBufferPtr                      *byte                (optional)
	// 	    localMemoryLength                         uint64               (optional)
	// 	    importedMemoryInstance                    *wasm.MemoryInstance (optional)
	// 	    importedFunctions [len(vm.importedFunctions)] struct { the total size depends on # of imported functions.
	// 	        executable      *byte
	// 	        opaqueCtx       *moduleContextOpaque
	// 	    }
	// 	    TODO: add more fields, like tables and globals
	// 	}
	//
	// See wazevoapi.NewModuleContextOffsetData for the details of the offsets.
	moduleContextOpaque []byte
)

func (m *moduleEngine) setupOpaque() {
	inst := m.module
	offsets := &m.parent.offsets
	opaque := m.opaque

	if lm := offsets.LocalMemoryBegin; lm >= 0 {
		b := uint64(uintptr(unsafe.Pointer(&inst.MemoryInstance.Buffer[0])))
		s := uint64(len(inst.MemoryInstance.Buffer))
		binary.LittleEndian.PutUint64(opaque[lm:], b)
		binary.LittleEndian.PutUint64(opaque[lm+8:], s)
	}

	if im := offsets.ImportedMemoryBegin; im >= 0 {
		b := uint64(uintptr(unsafe.Pointer(&inst.MemoryInstance)))
		binary.LittleEndian.PutUint64(opaque[im:], b)
	}

	// Note: imported functions are resolved in ResolveImportedFunction.
}

// NewFunction implements wasm.ModuleEngine.
func (m *moduleEngine) NewFunction(index wasm.Index) api.Function {
	localIndex := index
	if importedFnCount := m.module.Source.ImportFunctionCount; index < importedFnCount {
		panic("TODO: directly call a imported function.")
	} else {
		localIndex -= importedFnCount
	}

	src := m.module.Source
	typ := src.TypeSection[src.FunctionSection[localIndex]]
	sizeOfParamResultSlice := len(typ.Results)
	if ps := len(typ.Params); ps > sizeOfParamResultSlice {
		sizeOfParamResultSlice = ps
	}
	p := m.parent
	offset := p.functionOffsets[localIndex]

	ce := &callEngine{
		indexInModule:          index,
		executable:             &p.executable[offset.offset],
		parent:                 m,
		sizeOfParamResultSlice: sizeOfParamResultSlice,
	}
	ce.init()
	return ce
}

// ResolveImportedFunction implements wasm.ModuleEngine.
func (m *moduleEngine) ResolveImportedFunction(index, indexInImportedModule wasm.Index, importedModuleEngine wasm.ModuleEngine) {
	ptr, moduleCtx := m.parent.offsets.ImportedFunctionOffset(index)
	importedME := importedModuleEngine.(*moduleEngine)

	offset := importedME.parent.functionOffsets[indexInImportedModule]
	// When calling imported function from the machine code, we need to skip the Go preamble.
	executable := &importedME.parent.executable[offset.offset+offset.goPreambleSize]
	binary.LittleEndian.PutUint64(m.opaque[ptr:], uint64(uintptr(unsafe.Pointer(executable))))
	binary.LittleEndian.PutUint64(m.opaque[moduleCtx:], uint64(uintptr(unsafe.Pointer(importedME.opaquePtr))))
}

// DoneInstantiation implements wasm.ModuleEngine.
func (m *moduleEngine) DoneInstantiation() {
	m.setupOpaque()
}

// LookupFunction implements wasm.ModuleEngine.
func (m *moduleEngine) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (api.Function, error) {
	panic("TODO")
}

// FunctionInstanceReference implements wasm.ModuleEngine.
func (m *moduleEngine) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference { panic("TODO") }
