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
		opaquePtr              *byte
		parent                 *compiledModule
		module                 *wasm.ModuleInstance
		opaque                 moduleContextOpaque
		localFunctionInstances []*functionInstance
		importedFunctions      []importedFunction
	}

	functionInstance struct {
		executable             *byte
		moduleContextOpaquePtr *byte
		typeID                 wasm.FunctionTypeID
	}

	importedFunction struct {
		me            *moduleEngine
		indexInModule wasm.Index
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
	// 	    importedFunctions                         [# of importedFunctions]functionInstance
	//      globals                                   []*wasm.GlobalInstance (optional)
	//      typeIDsBegin                              &wasm.ModuleInstance.TypeIDs[0]  (optional)
	//      tables                                    []*wasm.TableInstance  (optional)
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
			binary.LittleEndian.PutUint64(opaque[globalOffset:], uint64(uintptr(unsafe.Pointer(g))))
			globalOffset += 8
		}
	}

	if tableOffset := offsets.TablesBegin; tableOffset >= 0 {
		// First we write the first element's address of typeIDs.
		binary.LittleEndian.PutUint64(opaque[offsets.TypeIDs1stElement:], uint64(uintptr(unsafe.Pointer(&inst.TypeIDs[0]))))

		// Then we write the table addresses.
		for _, table := range inst.Tables {
			binary.LittleEndian.PutUint64(opaque[tableOffset:], uint64(uintptr(unsafe.Pointer(table))))
			tableOffset += 8
		}
	}
}

// NewFunction implements wasm.ModuleEngine.
func (m *moduleEngine) NewFunction(index wasm.Index) api.Function {
	localIndex := index
	if importedFnCount := m.module.Source.ImportFunctionCount; index < importedFnCount {
		imported := &m.importedFunctions[index]
		return imported.me.NewFunction(imported.indexInModule)
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
		numberOfResults:        typ.ResultNumInUint64,
	}

	ce.execCtx.memoryGrowTrampolineAddress = &m.parent.builtinFunctions.memoryGrowExecutable[0]
	ce.execCtx.stackGrowCallSequenceAddress = &m.parent.builtinFunctions.stackGrowExecutable[0]
	ce.init()
	return ce
}

// ResolveImportedFunction implements wasm.ModuleEngine.
func (m *moduleEngine) ResolveImportedFunction(index, indexInImportedModule wasm.Index, importedModuleEngine wasm.ModuleEngine) {
	executableOffset, moduleCtxOffset, typeIDOffset := m.parent.offsets.ImportedFunctionOffset(index)
	importedME := importedModuleEngine.(*moduleEngine)

	offset := importedME.parent.functionOffsets[indexInImportedModule]
	typeID := getTypeIDOf(indexInImportedModule, importedME.module)
	// When calling imported function from the machine code, we need to skip the Go preamble.
	executable := &importedME.parent.executable[offset.offset+offset.goPreambleSize]
	// Write functionInstance.
	binary.LittleEndian.PutUint64(m.opaque[executableOffset:], uint64(uintptr(unsafe.Pointer(executable))))
	binary.LittleEndian.PutUint64(m.opaque[moduleCtxOffset:], uint64(uintptr(unsafe.Pointer(importedME.opaquePtr))))
	binary.LittleEndian.PutUint64(m.opaque[typeIDOffset:], uint64(typeID))

	// Write importedFunction so that it can be used by NewFunction.
	m.importedFunctions[index] = importedFunction{me: importedME, indexInModule: indexInImportedModule}
}

func getTypeIDOf(funcIndex wasm.Index, m *wasm.ModuleInstance) wasm.FunctionTypeID {
	source := m.Source

	var typeIndex wasm.Index
	if funcIndex >= source.ImportFunctionCount {
		funcIndex -= source.ImportFunctionCount
		typeIndex = source.FunctionSection[funcIndex]
	} else {
		var cnt wasm.Index
		for i := range source.ImportSection {
			if source.ImportSection[i].Type == wasm.ExternTypeFunc {
				if cnt == funcIndex {
					typeIndex = source.ImportSection[i].DescFunc
					break
				}
				cnt++
			}
		}
	}
	return m.TypeIDs[typeIndex]
}

// ResolveImportedMemory implements wasm.ModuleEngine.
func (m *moduleEngine) ResolveImportedMemory(importedModuleEngine wasm.ModuleEngine) {
	importedME := importedModuleEngine.(*moduleEngine)
	inst := importedME.module

	var memInstPtr uint64
	var memOwnerOpaquePtr uint64
	if offs := importedME.parent.offsets; offs.ImportedMemoryBegin >= 0 {
		offset := offs.ImportedMemoryBegin
		memInstPtr = binary.LittleEndian.Uint64(importedME.opaque[offset:])
		memOwnerOpaquePtr = binary.LittleEndian.Uint64(importedME.opaque[offset+8:])
	} else {
		memInstPtr = uint64(uintptr(unsafe.Pointer(inst.MemoryInstance)))
		memOwnerOpaquePtr = uint64(uintptr(unsafe.Pointer(importedME.opaquePtr)))
	}
	offset := m.parent.offsets.ImportedMemoryBegin
	binary.LittleEndian.PutUint64(m.opaque[offset:], memInstPtr)
	binary.LittleEndian.PutUint64(m.opaque[offset+8:], memOwnerOpaquePtr)
}

// DoneInstantiation implements wasm.ModuleEngine.
func (m *moduleEngine) DoneInstantiation() {
	if !m.module.Source.IsHostModule {
		m.setupOpaque()
	}
}

// FunctionInstanceReference implements wasm.ModuleEngine.
func (m *moduleEngine) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference {
	if funcIndex < m.module.Source.ImportFunctionCount {
		begin, _, _ := m.parent.offsets.ImportedFunctionOffset(funcIndex)
		return uintptr(unsafe.Pointer(&m.opaque[begin]))
	}

	p := m.parent
	executable := &p.executable[p.functionOffsets[funcIndex].offset]
	typeID := m.module.TypeIDs[m.module.Source.FunctionSection[funcIndex]]

	lf := &functionInstance{
		executable:             executable,
		moduleContextOpaquePtr: m.opaquePtr,
		typeID:                 typeID,
	}
	m.localFunctionInstances = append(m.localFunctionInstances, lf)
	return uintptr(unsafe.Pointer(lf))
}

// LookupFunction implements wasm.ModuleEngine.
func (m *moduleEngine) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (*wasm.ModuleInstance, wasm.Index) {
	panic("TODO")
}
