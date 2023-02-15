package wasm

import (
	"github.com/tetratelabs/wazero/api"
	"unsafe"
)

func (m *Module) ModuleID() ModuleID {
	return m.ID
}

func (m *Module) TypeCounts() uint32 {
	return uint32(len(m.TypeSection))
}

func (m *Module) Type(i Index) (params, results []api.ValueType) {
	typ := m.TypeSection[i]
	return typ.Params, typ.Results
}

func (m *Module) FuncTypeIndex(funcIndex Index) (typeIndex Index) {
	importedCount := m.ImportFuncCount()
	if funcIndex < importedCount {
		panic("TODO")
	}
	return m.FunctionSection[funcIndex-importedCount]
}

func (m *Module) HostModule() bool {
	return m.IsHostModule
}

func (m *Module) LocalMemoriesCount() uint32 {
	if m.MemorySection != nil {
		return 1
	} else {
		return 0
	}
}

func (m *Module) ImportedMemoriesCount() uint32 {
	return uint32(len(m.ImportedMemories()))
}

func (m *Module) MemoryMinMax() (min, max uint32, ok bool) {
	if m.MemorySection == nil {
		imported := m.ImportedMemories()
		if len(imported) == 0 {
			return
		}
		memDef := imported[0]
		min, max = memDef.Min(), memDef.Min()
	} else {
		min, max = m.MemorySection.Min, m.MemorySection.Max
	}
	ok = true
	return
}

func (m *Module) CodeCount() uint32 {
	return uint32(len(m.CodeSection))
}

func (m *Module) CodeAt(i Index) (localTypes, body []byte) {
	c := m.CodeSection[i]
	return c.LocalTypes, c.Body
}

func (m *ModuleInstance) ModuleInstanceName() string {
	return m.Name
}

func (m *ModuleInstance) ImportedFunctions() (moduleInstances []any, indexes []Index) {
	for _, f := range m.Functions {
		if f.Module == m {
			break
		} else {
			moduleInstances = append(moduleInstances, f.Module)
			indexes = append(indexes, f.Idx)
		}
	}
	return
}

func (m *ModuleInstance) MemoryInstanceBuffer() []byte {
	if m.Memory != nil {
		return m.Memory.Buffer
	}
	return nil
}

func (m *ModuleInstance) ImportedMemoryInstancePtr() uintptr {
	if m.Memory != nil {
		return uintptr(unsafe.Pointer(m.Memory))
	}
	return 0
}

func (f FunctionInstance) ModuleInstanceName() string {
	return f.Module.ModuleInstanceName()
}

func (f FunctionInstance) FunctionType() ([]api.ValueType, []api.ValueType) {
	typ := f.Type
	return typ.Params, typ.Results
}

func (f FunctionInstance) Index() Index {
	return f.Idx
}
