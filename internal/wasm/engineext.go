package wasm

import (
	"unsafe"

	"github.com/tetratelabs/wazero/api"
)

// ModuleID implements engineext.Module.
func (m *Module) ModuleID() ModuleID {
	return m.ID
}

// TypeCounts implements engineext.Module.
func (m *Module) TypeCounts() uint32 {
	return uint32(len(m.TypeSection))
}

// Type implements engineext.Module.
func (m *Module) Type(i Index) (params, results []api.ValueType) {
	typ := m.TypeSection[i]
	return typ.Params, typ.Results
}

// FuncTypeIndex implements engineext.Module.
func (m *Module) FuncTypeIndex(funcIndex Index) (typeIndex Index) {
	importedCount := m.ImportFuncCount()
	if funcIndex < importedCount {
		panic("TODO")
	}
	return m.FunctionSection[funcIndex-importedCount]
}

// HostModule implements engineext.Module.
func (m *Module) HostModule() bool {
	return m.IsHostModule
}

// LocalMemoriesCount implements engineext.Module.
func (m *Module) LocalMemoriesCount() uint32 {
	if m.MemorySection != nil {
		return 1
	} else {
		return 0
	}
}

// ImportedMemoriesCount implements engineext.Module.
func (m *Module) ImportedMemoriesCount() uint32 {
	return uint32(len(m.ImportedMemories()))
}

// MemoryMinMax implements engineext.Module.
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

// CodeCount implements engineext.Module.
func (m *Module) CodeCount() uint32 {
	return uint32(len(m.CodeSection))
}

// CodeAt implements engineext.Module.
func (m *Module) CodeAt(i Index) (localTypes, body []byte) {
	c := m.CodeSection[i]
	return c.LocalTypes, c.Body
}

// ModuleInstanceName implements engineext.ModuleInstance.
func (m *ModuleInstance) ModuleInstanceName() string {
	return m.Name
}

// ImportedFunctions implements engineext.ModuleInstance.
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

// MemoryInstanceBuffer implements engineext.ModuleInstance.
func (m *ModuleInstance) MemoryInstanceBuffer() []byte {
	if m.Memory != nil {
		return m.Memory.Buffer
	}
	return nil
}

// ImportedMemoryInstancePtr implements engineext.ModuleInstance.
func (m *ModuleInstance) ImportedMemoryInstancePtr() uintptr {
	if m.Memory != nil {
		return uintptr(unsafe.Pointer(m.Memory))
	}
	return 0
}

// ModuleInstanceName implements engineext.FunctionInstance.
func (f FunctionInstance) ModuleInstanceName() string {
	return f.Module.ModuleInstanceName()
}

// FunctionType implements engineext.FunctionInstance.
func (f FunctionInstance) FunctionType() ([]api.ValueType, []api.ValueType) {
	typ := f.Type
	return typ.Params, typ.Results
}

// Index implements engineext.FunctionInstance.
func (f FunctionInstance) Index() Index {
	return f.Idx
}
