package internalwasm

import (
	"bytes"
	"fmt"
	"math"
	"reflect"

	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

type (

	// ModuleInstance represents instantiated wasm module.
	// The difference from the spec is that in wazero, a ModuleInstance holds pointers
	// to the instances, rather than "addresses" (i.e. index to Store.Functions, Globals, etc) for convenience.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-moduleinst
	ModuleInstance struct {
		Name      string
		Exports   map[string]*ExportInstance
		Functions []*FunctionInstance
		Globals   []*GlobalInstance
		// MemoryInstance is set when Module.MemorySection had a memory, regardless of whether it was exported.
		// Note: This avoids the name "Memory" which is an interface method name.
		MemoryInstance *MemoryInstance
		TableInstance  *TableInstance
		Types          []*TypeInstance

		// TODO
		refCount      int
		moduleImports map[*ModuleInstance]struct{}
	}

	// ExportInstance represents an exported instance in a Store.
	// The difference from the spec is that in wazero, a ExportInstance holds pointers
	// to the instances, rather than "addresses" (i.e. index to Store.Functions, Globals, etc) for convenience.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-exportinst
	ExportInstance struct {
		Type     ExternType
		Function *FunctionInstance
		Global   *GlobalInstance
		Memory   *MemoryInstance
		Table    *TableInstance
	}

	// FunctionInstance represents a function instance in a Store.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0
	FunctionInstance struct {
		// ModuleInstance holds the pointer to the module instance to which this function belongs.
		ModuleInstance *ModuleInstance
		// Body is the function body in WebAssembly Binary Format
		Body []byte
		// FunctionType holds the pointer to TypeInstance whose functionType field equals that of this function.
		FunctionType *TypeInstance
		// LocalTypes holds types of locals.
		LocalTypes []ValueType
		// FunctionKind describes how this function should be called.
		FunctionKind FunctionKind
		// HostFunction holds the runtime representation of host functions.
		// This is nil when FunctionKind == FunctionKindWasm. Otherwise, all the above fields are ignored as they are
		// specific to Wasm functions.
		HostFunction *reflect.Value
		// Index is the index of this function instance in store.Functions, and is exported because
		// all function calls are made via funcaddr at runtime, not the index (scoped to a module).
		//
		// This is used by both host and non-host functions.
		Index FunctionIndex
		// Name is for debugging purpose, and is used to argument the stack traces.
		//
		// When HostFunction is not nil, this returns dot-delimited parameters given to
		// Store.AddHostFunction. Ex. something.realistic
		//
		// Otherwise, this is the corresponding value in NameSection.FunctionNames or "unknown" if unavailable.
		Name string
	}

	// TypeInstance is a store-specific representation of FunctionType where the function type
	// is coupled with TypeID which is specific in a store.
	TypeInstance struct {
		Type *FunctionType
		// TypeID is assigned by a store for FunctionType.
		TypeID FunctionTypeID
	}

	// GlobalInstance represents a global instance in a store.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-instances%E2%91%A0
	GlobalInstance struct {
		Type *GlobalType
		// Val holds a 64-bit representation of the actual value.
		Val   uint64
		index globalIndex
	}

	// TableInstance represents a table instance in a store.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-instances%E2%91%A0
	//
	// Note this is fixed to function type until post 20191205 reference type is implemented.
	TableInstance struct {
		// Table holds the table elements managed by this table instance.
		//
		// Note: we intentionally use "[]TableElement", not "[]*TableElement",
		// because the JIT Engine accesses this slice directly from assembly.
		// If pointer type is used, the access becomes two level indirection (two hops of pointer jumps)
		// which is a bit costly. TableElement is 96 bit (32 and 64 bit fields) so the cost of using value type
		// would be ignorable.
		Table []TableElement
		Min   uint32
		Max   *uint32
		// Currently fixed to 0x70 (funcref type).
		ElemType byte
		index    tableIndex
	}

	// TableElement represents an item in a table instance.
	//
	// Note: this is fixed to function type as it is the only supported type in WebAssembly 1.0 (20191205)
	TableElement struct {
		// FunctionIndex is funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr)
		// of the target function instance. More precisely, this equals the index of
		// the target function instance in Store.FunctionInstances.
		FunctionIndex FunctionIndex
		// FunctionTypeID is the type ID of the target function's type, which
		// equals store.Functions[FunctionIndex].FunctionType.TypeID.
		FunctionTypeID FunctionTypeID
	}

	// MemoryInstance represents a memory instance in a store, and implements wasm.Memory.
	//
	// Note: In WebAssembly 1.0 (20191205), there may be up to one Memory per store, which means the precise memory is always
	// wasm.Store Memories index zero: `store.Memories[0]`
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0.
	MemoryInstance struct {
		Buffer []byte
		Min    uint32
		Max    *uint32
		index  memoryIndex
	}

	// FunctionIndex is funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr),
	// and the index to Store.Functions.
	FunctionIndex storeIndex
	// memoryIndex is memaddr in the spec(https://www.w3.org/TR/wasm-core-1/#syntax-memaddr),
	// and the index to Store.Memories.
	memoryIndex storeIndex
	// globalIndex is memaddr (https://www.w3.org/TR/wasm-core-1/#syntax-globaladdr),
	// and the index to Store.Globals.
	globalIndex storeIndex
	// tableIndex is tableaddr (https://www.w3.org/TR/wasm-core-1/#syntax-tableaddr),
	// and the index to Store.Tables.
	tableIndex storeIndex

	// storeIndex represents the offset in of an instance in a store.
	storeIndex uint64

	// FunctionTypeID is a uniquely assigned integer for a function type.
	// This is wazero specific runtime object and specific to a store,
	// and used at runtime to do type-checks on indirect function calls.
	FunctionTypeID uint32
)

// newModuleInstance bundles all the instances for a module and creates a new module instance.
func newModuleInstance(name string, module *Module, importedFunctions, functions []*FunctionInstance,
	importedGlobals, globals []*GlobalInstance, importedTable, table *TableInstance,
	memory, importedMemory *MemoryInstance, typeInstances []*TypeInstance, moduleImports map[*ModuleInstance]struct{}) *ModuleInstance {

	instance := &ModuleInstance{Name: name, Types: typeInstances, moduleImports: moduleImports}

	instance.Functions = append(instance.Functions, importedFunctions...)
	for i, f := range functions {
		// Associate each function with the type instance and the module instance's pointer.
		f.FunctionType = typeInstances[module.FunctionSection[i]]
		f.ModuleInstance = instance
		instance.Functions = append(instance.Functions, f)
	}

	instance.Globals = append(instance.Globals, importedGlobals...)
	instance.Globals = append(instance.Globals, globals...)

	if importedTable != nil {
		instance.TableInstance = importedTable
	} else {
		instance.TableInstance = table
	}

	if importedMemory != nil {
		instance.MemoryInstance = importedMemory
	} else {
		instance.MemoryInstance = memory
	}

	instance.buildExportInstances(module.ExportSection)
	return instance
}

func (m *ModuleInstance) buildExportInstances(exports map[string]*Export) {
	m.Exports = make(map[string]*ExportInstance, len(exports))
	for _, exp := range exports {
		index := exp.Index
		var ei *ExportInstance
		switch exp.Type {
		case ExternTypeFunc:
			ei = &ExportInstance{Type: exp.Type, Function: m.Functions[index]}
			// The module instance of the host function is a fake that only includes the function and its types.
			// We need to assign the ModuleInstance when re-exporting so that any memory defined in the target is
			// available to the wasm.ModuleContext Memory.
			if ei.Function.HostFunction != nil {
				ei.Function.ModuleInstance = m
			}
		case ExternTypeGlobal:
			ei = &ExportInstance{Type: exp.Type, Global: m.Globals[index]}
		case ExternTypeMemory:
			ei = &ExportInstance{Type: exp.Type, Memory: m.MemoryInstance}
		case ExternTypeTable:
			ei = &ExportInstance{Type: exp.Type, Table: m.TableInstance}
		}

		// We already validated the duplicates during module validation phase.
		_ = m.addExport(exp.Name, ei)
	}
}

func (m *ModuleInstance) validateData(data []*DataSegment) (err error) {
	for _, d := range data {
		offset := int(executeConstExpression(m.Globals, d.OffsetExpression).(int32))

		ceil := offset + len(d.Init)
		if offset < 0 || ceil > len(m.MemoryInstance.Buffer) {
			return fmt.Errorf("out of bounds memory access")
		}
	}
	return
}

func (m *ModuleInstance) applyData(data []*DataSegment) {
	for _, d := range data {
		offset := executeConstExpression(m.Globals, d.OffsetExpression).(int32)
		copy(m.MemoryInstance.Buffer[offset:], d.Init)
	}
}

func (m *ModuleInstance) validateElements(elements []*ElementSegment) (err error) {
	for _, elem := range elements {
		offset := int(executeConstExpression(m.Globals, elem.OffsetExpr).(int32))
		ceil := offset + len(elem.Init)

		if offset < 0 || ceil > len(m.TableInstance.Table) {
			return fmt.Errorf("out of bounds table access")
		}
		for _, elm := range elem.Init {
			if elm >= uint32(len(m.Functions)) {
				return fmt.Errorf("unknown function specified by element")
			}
		}
	}
	return
}

func (m *ModuleInstance) applyElements(elements []*ElementSegment) {
	for _, elem := range elements {
		offset := int(executeConstExpression(m.Globals, elem.OffsetExpr).(int32))
		table := m.TableInstance.Table
		for i, elm := range elem.Init {
			pos := i + offset
			targetFunc := m.Functions[elm]
			table[pos] = TableElement{
				FunctionIndex:  targetFunc.Index,
				FunctionTypeID: targetFunc.FunctionType.TypeID,
			}
		}
	}
}

// addExport adds and indexes the given export or errs if the name is already exported.
func (m *ModuleInstance) addExport(name string, e *ExportInstance) error {
	if _, ok := m.Exports[name]; ok {
		return fmt.Errorf("%q is already exported in module %q", name, m.Name)
	}
	m.Exports[name] = e
	return nil
}

// GetExport returns an export of the given name and type or errs if not exported or the wrong type.
func (m *ModuleInstance) GetExport(name string, et ExternType) (*ExportInstance, error) {
	exp, ok := m.Exports[name]
	if !ok {
		return nil, fmt.Errorf("%q is not exported in module %q", name, m.Name)
	}
	if exp.Type != et {
		return nil, fmt.Errorf("export %q in module %q is a %s, not a %s", name, m.Name, ExternTypeName(exp.Type), ExternTypeName(et))
	}
	return exp, nil
}

func executeConstExpression(globals []*GlobalInstance, expr *ConstantExpression) (v interface{}) {
	r := bytes.NewBuffer(expr.Data)
	switch expr.Opcode {
	case OpcodeI32Const:
		v, _, _ = leb128.DecodeInt32(r)
	case OpcodeI64Const:
		v, _, _ = leb128.DecodeInt64(r)
	case OpcodeF32Const:
		v, _ = ieee754.DecodeFloat32(r)
	case OpcodeF64Const:
		v, _ = ieee754.DecodeFloat64(r)
	case OpcodeGlobalGet:
		id, _, _ := leb128.DecodeUint32(r)
		g := globals[id]
		switch g.Type.ValType {
		case ValueTypeI32:
			v = int32(g.Val)
		case ValueTypeI64:
			v = int64(g.Val)
		case ValueTypeF32:
			v = publicwasm.DecodeF32(g.Val)
		case ValueTypeF64:
			v = publicwasm.DecodeF64(g.Val)
		}
	}
	return
}

func newTableInstance(min uint32, max *uint32) *TableInstance {
	tableInst := &TableInstance{
		Table:    make([]TableElement, min),
		Min:      min,
		Max:      max,
		ElemType: 0x70, // funcref
	}
	for i := range tableInst.Table {
		tableInst.Table[i] = TableElement{
			FunctionTypeID: UninitializedTableElementTypeID,
		}
	}
	return tableInst
}

// UninitializedTableElementTypeID math.MaxUint32 to represent the uninitialized elements.
const UninitializedTableElementTypeID FunctionTypeID = math.MaxUint32
