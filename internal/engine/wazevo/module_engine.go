package wazevo

import (
	"encoding/binary"
	"unsafe"

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
	//
	// Internally, the buffer is structured as follows:
	//
	// 	type moduleContextOpaque struct {
	// 	    moduleInstance                            *wasm.ModuleInstance
	// 	    localMemoryBufferPtr                      *byte                (optional)
	// 	    localMemoryLength                         uint64               (optional)
	// 	    importedMemoryInstance                    *wasm.MemoryInstance (optional)
	// 	    importedMemoryOwnerOpaqueCtx              *byte                (optional)
	// 	    importedFunctions [importedFunctions] struct { the total size depends on # of imported functions.
	// 	        executable      *byte
	// 	        opaqueCtx       *moduleContextOpaque
	// 	    }
	//      globals                                    []*wasm.GlobalInstance (optional)
	// 	    TODO: add more fields, like tables, etc.
	// 	}
	//
	// See wazevoapi.NewModuleContextOffsetData for the details of the offsets.
	//
	// Note that for host modules, the structure is entirely different. See buildHostModuleOpaque.
	moduleContextOpaque []byte
)

func putLocalMemory(opaque []byte, offset wazevoapi.Offset, mem *wasm.MemoryInstance) {
	b := uint64(uintptr(unsafe.Pointer(&mem.Buffer[0])))
	s := uint64(len(mem.Buffer))
	binary.LittleEndian.PutUint64(opaque[offset:], b)
	binary.LittleEndian.PutUint64(opaque[offset+8:], s)
}

func (m *moduleEngine) setupOpaque() {
	inst := m.module
	offsets := &m.parent.offsets
	opaque := m.opaque

	binary.LittleEndian.PutUint64(opaque[offsets.ModuleInstanceOffset:],
		uint64(uintptr(unsafe.Pointer(m.module))),
	)

	if lm := offsets.LocalMemoryBegin; lm >= 0 {
		putLocalMemory(opaque, lm, inst.MemoryInstance)
	}

	// Note: imported memory is resolved in ResolveImportedFunction.

	// Note: imported functions are resolved in ResolveImportedFunction.

	if globalOffset := offsets.GlobalsBegin; globalOffset >= 0 {
		for _, g := range inst.Globals {
			b := uint64(uintptr(unsafe.Pointer(g)))
			binary.LittleEndian.PutUint64(opaque[globalOffset:], b)
			globalOffset += 8
		}
	}
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

	ce.execCtx.memoryGrowTrampolineAddress = &m.parent.builtinFunctions.memoryGrowExecutable[0]
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

// ResolveImportedMemory implements wasm.ModuleEngine.
func (m *moduleEngine) ResolveImportedMemory(importedModuleEngine wasm.ModuleEngine) {
	importedME := importedModuleEngine.(*moduleEngine)
	inst := importedME.module

	if importedME.parent.offsets.ImportedMemoryBegin >= 0 {
		// This case can be resolved by recursively resolving the owner.
		panic("TODO: support re-exported memory import")
	}

	offset := m.parent.offsets.ImportedMemoryBegin
	b := uint64(uintptr(unsafe.Pointer(inst.MemoryInstance)))
	binary.LittleEndian.PutUint64(m.opaque[offset:], b)
	binary.LittleEndian.PutUint64(m.opaque[offset+8:], uint64(uintptr(unsafe.Pointer(importedME.opaquePtr))))
}

// DoneInstantiation implements wasm.ModuleEngine.
func (m *moduleEngine) DoneInstantiation() {
	if !m.module.Source.IsHostModule {
		m.setupOpaque()
	}
}

// LookupFunction implements wasm.ModuleEngine.
func (m *moduleEngine) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (api.Function, error) {
	panic("TODO")
}

// FunctionInstanceReference implements wasm.ModuleEngine.
func (m *moduleEngine) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference { panic("TODO") }
