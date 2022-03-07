package internalwasm

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/tetratelabs/wazero/internal/ieee754"
	"github.com/tetratelabs/wazero/internal/leb128"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

type (
	// Store is the runtime representation of "instantiated" Wasm module and objects.
	// Multiple modules can be instantiated within a single store, and each instance,
	// (e.g. function instance) can be referenced by other module instances in a Store via Module.ImportSection.
	//
	// Every type whose name ends with "Instance" suffix belongs to exactly one store.
	//
	// Note that store is not thread (concurrency) safe, meaning that using single Store
	// via multiple goroutines might result in race conditions. In that case, the invocation
	// and access to any methods and field of Store must be guarded by mutex.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#store%E2%91%A0
	Store struct {
		// The following fields are wazero-specific fields of Store.

		// ctx is the default context used for function calls
		ctx context.Context

		// Engine is a global context for a Store which is in responsible for compilation and execution of Wasm modules.
		engine Engine

		// EnabledFeatures are read-only to allow optimizations.
		EnabledFeatures Features

		// modules holds the instantiated Wasm modules by module name from Instantiate.
		modules map[string]*ModuleInstance

		// typeIDs maps each FunctionType.String() to a unique FunctionTypeID. This is used at runtime to
		// do type-checks on indirect function calls.
		typeIDs map[string]FunctionTypeID

		// maximumFunctionIndex represents the limit on the number of function addresses (= function instances) in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionIndex FunctionIndex

		//  maximumFunctionTypes represents the limit on the number of function types in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionTypes int

		// releasedFunctionIndex holds reusable FunctionIndexes. An index is added when
		// a function instance is released in releaseFunctions, and is popped when
		// a new instance is added in addFunctions.
		releasedFunctionIndex map[FunctionIndex]struct{}

		// functions holds function instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr).
		//
		// Note: Functions are held by store as well as ModuleInstances, in contrast to other instances (memory, table, and globals)
		// which are only owned by ModuleInstance. This is because the function call implementation in engines depend on storeIndex
		// of function instance (FunctionIndex).
		// TODO: decouple engine's function call implementation from store-wide context (in this case FunctionIndex), and remove
		// the necessity to hold FunctionInstances in store in order to reduce the mutex usage. Note that this might come with
		// the runtime overhead (e.g. adding additional instruction for each call instruction).
		functions []*FunctionInstance

		// mux is used to guard the fields from concurrent access.
		mux sync.RWMutex
	}

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
		// Memory is set when Module.MemorySection had a memory, regardless of whether it was exported.
		Memory *MemoryInstance
		Table  *TableInstance
		Types  []*TypeInstance

		// Ctx holds default function call context from this function instance.
		Ctx *ModuleContext

		// hostModule holds HostModule if this is a "host module" which is created in store.NewHostModule.
		hostModule *HostModule

		// mux is used to guard the fields from concurrent access.
		mux sync.Mutex

		// dependentCount is the current number of modules which import this module. On Store.ReleaseModule, this number
		// must be zero otherwise it fails.
		dependentCount int // guarded by mux

		// dependencies holds imported modules. This is used when releasing this module instance, or decrementing the
		// dependentCount of the imported modules.
		dependencies map[*ModuleInstance]struct{}
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
		// Name is for debugging purpose, and is used to argument the stack traces.
		//
		// When GoFunc is not nil, this returns dot-delimited parameters given to
		// Store.NewHostModule. Ex. something.realistic
		//
		// Otherwise, this is the corresponding value in NameSection.FunctionNames or "unknown" if unavailable.
		Name string

		// Kind describes how this function should be called.
		Kind FunctionKind

		// Type is the signature of this function.
		Type *FunctionType

		// LocalTypes holds types of locals, set when Kind == FunctionKindWasm
		LocalTypes []ValueType

		// Body is the function body in WebAssembly Binary Format, set when Kind == FunctionKindWasm
		Body []byte

		// GoFunc holds the runtime representation of host functions.
		// This is nil when Kind == FunctionKindWasm. Otherwise, all the above fields are ignored as they are
		// specific to Wasm functions.
		GoFunc *reflect.Value

		// Fields above here are settable prior to instantiation. Below are set by the Store during instantiation.

		// ModuleInstance holds the pointer to the module instance to which this function belongs.
		Module *ModuleInstance
		// TypeID is assigned by a store for FunctionType.
		TypeID FunctionTypeID
		// Index is the index of this function instance in Store.functions, and is exported because
		// all function calls are made via funcaddr at runtime, not the index (scoped to a module).
		//
		// This is used by both host and non-host functions.
		Index FunctionIndex
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
		Val uint64
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
		// equals store.Functions[FunctionIndex].TypeID.
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
	}

	// FunctionIndex is funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr),
	// and the index to Store.Functions.
	FunctionIndex storeIndex

	// storeIndex represents the offset in of an instance in a store.
	storeIndex uint64

	// FunctionTypeID is a uniquely assigned integer for a function type.
	// This is wazero specific runtime object and specific to a store,
	// and used at runtime to do type-checks on indirect function calls.
	FunctionTypeID uint32
)

// The wazero specific limitations described at RATIONALE.md.
const (
	maximumFunctionIndex = 1 << 27
	maximumFunctionTypes = 1 << 27
)

// newModuleInstance bundles all the instances for a module and creates a new module instance.
func newModuleInstance(name string, module *Module, importedFunctions, functions []*FunctionInstance,
	importedGlobals, globals []*GlobalInstance, importedTable, table *TableInstance,
	memory, importedMemory *MemoryInstance, typeInstances []*TypeInstance, moduleImports map[*ModuleInstance]struct{}) *ModuleInstance {

	instance := &ModuleInstance{Name: name, Types: typeInstances, dependencies: moduleImports}

	instance.Functions = append(instance.Functions, importedFunctions...)
	for i, f := range functions {
		// Associate each function with the type instance and the module instance's pointer.
		f.Module = instance
		f.TypeID = typeInstances[module.FunctionSection[i]].TypeID
		instance.Functions = append(instance.Functions, f)
	}

	instance.Globals = append(instance.Globals, importedGlobals...)
	instance.Globals = append(instance.Globals, globals...)

	if importedTable != nil {
		instance.Table = importedTable
	} else {
		instance.Table = table
	}

	if importedMemory != nil {
		instance.Memory = importedMemory
	} else {
		instance.Memory = memory
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
			if ei.Function.GoFunc != nil {
				ei.Function.Module = m
			}
		case ExternTypeGlobal:
			ei = &ExportInstance{Type: exp.Type, Global: m.Globals[index]}
		case ExternTypeMemory:
			ei = &ExportInstance{Type: exp.Type, Memory: m.Memory}
		case ExternTypeTable:
			ei = &ExportInstance{Type: exp.Type, Table: m.Table}
		}

		// We already validated the duplicates during module validation phase.
		m.Exports[exp.Name] = ei
	}
}

func (m *ModuleInstance) validateData(data []*DataSegment) (err error) {
	for _, d := range data {
		offset := int(executeConstExpression(m.Globals, d.OffsetExpression).(int32))

		ceil := offset + len(d.Init)
		if offset < 0 || ceil > len(m.Memory.Buffer) {
			return fmt.Errorf("out of bounds memory access")
		}
	}
	return
}

func (m *ModuleInstance) applyData(data []*DataSegment) {
	for _, d := range data {
		offset := executeConstExpression(m.Globals, d.OffsetExpression).(int32)
		copy(m.Memory.Buffer[offset:], d.Init)
	}
}

func (m *ModuleInstance) validateElements(elements []*ElementSegment) (err error) {
	for _, elem := range elements {
		offset := int(executeConstExpression(m.Globals, elem.OffsetExpr).(int32))
		ceil := offset + len(elem.Init)

		if offset < 0 || ceil > len(m.Table.Table) {
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
		table := m.Table.Table
		for i, elm := range elem.Init {
			pos := i + offset
			targetFunc := m.Functions[elm]
			table[pos] = TableElement{
				FunctionIndex:  targetFunc.Index,
				FunctionTypeID: targetFunc.TypeID,
			}
		}
	}
}

// GetExport returns an export of the given name and type or errs if not exported or the wrong type.
func (m *ModuleInstance) getExport(name string, et ExternType) (*ExportInstance, error) {
	exp, ok := m.Exports[name]
	if !ok {
		return nil, fmt.Errorf("%q is not exported in module %q", name, m.Name)
	}
	if exp.Type != et {
		return nil, fmt.Errorf("export %q in module %q is a %s, not a %s", name, m.Name, ExternTypeName(exp.Type), ExternTypeName(et))
	}
	return exp, nil
}

func NewStore(ctx context.Context, engine Engine, enabledFeatures Features) *Store {
	return &Store{
		ctx:                   ctx,
		engine:                engine,
		EnabledFeatures:       enabledFeatures,
		modules:               map[string]*ModuleInstance{},
		typeIDs:               map[string]FunctionTypeID{},
		maximumFunctionIndex:  maximumFunctionIndex,
		maximumFunctionTypes:  maximumFunctionTypes,
		releasedFunctionIndex: map[FunctionIndex]struct{}{},
	}
}

// checkFunctionIndexOverflow checks if there would be too many function instances in a store.
func (s *Store) checkFunctionIndexOverflow(newInstanceNum int) error {
	s.mux.RLock()
	defer s.mux.RUnlock()
	if len(s.functions) > int(s.maximumFunctionIndex)-newInstanceNum {
		return fmt.Errorf("too many functions in a store")
	}
	return nil
}

func (s *Store) Instantiate(module *Module, name string) (*PublicModule, error) {
	// Note: we do not take lock here in order to enable concurrent instantiation and compilation
	// of multiuple modules. When necessary, we take read or write locks in each method of store used here.

	if err := s.requireModuleUnused(name); err != nil {
		return nil, err
	}

	if err := s.checkFunctionIndexOverflow(len(module.FunctionSection)); err != nil {
		return nil, err
	}

	types, err := s.getTypes(module.TypeSection)
	if err != nil {
		return nil, err
	}

	moduleImportsFinalized := false
	importedFunctions, importedGlobals, importedTable, importedMemory, moduleImports, err := s.resolveImports(module)
	defer func() {
		if !moduleImportsFinalized {
			for moduleImport := range moduleImports {
				moduleImport.decDependentCount()
			}
		}
	}()
	if err != nil {
		return nil, err
	}

	functions, globals, table, memory :=
		module.buildFunctions(), module.buildGlobals(importedGlobals), module.buildTable(), module.buildMemory()

	// Now we have all instances from imports and local ones, so ready to create a new ModuleInstance.
	instance := newModuleInstance(name, module, importedFunctions, functions, importedGlobals,
		globals, importedTable, table, importedMemory, memory, types, moduleImports)

	if err = instance.validateElements(module.ElementSection); err != nil {
		return nil, err
	}

	if err := instance.validateData(module.DataSection); err != nil {
		return nil, err
	}

	// Now we are ready to compile functions.
	s.addFunctions(functions...) // Need to assign funcaddr to each instance before compilation.
	for i, f := range functions {
		// TODO: maybe better consider spawning multiple goroutines for compilations to accelerate.
		if err := s.engine.Compile(f); err != nil {
			// On the failure, release the assigned funcaddr and already compiled functions.
			if err := s.releaseFunctions(functions[:i]...); err != nil {
				return nil, err
			}
			idx := module.SectionElementCount(SectionIDFunction) - 1
			return nil, fmt.Errorf("compilation failed at index %d/%d: %w", i, idx, err)
		}
	}

	// Now all the validation passes, we are safe to mutate memory/table instances (possibly imported ones).
	instance.applyElements(module.ElementSection)
	instance.applyData(module.DataSection)

	// Persist the module instance.
	s.addModule(instance)

	// Plus, we can finalize the module import reference count.
	moduleImportsFinalized = true

	// Execute the start function.
	if module.StartSection != nil {
		funcIdx := *module.StartSection
		if _, err := s.engine.Call(instance.Ctx, instance.Functions[funcIdx]); err != nil {
			return nil, fmt.Errorf("module[%s] start function failed: %w", name, err)
		}
	}
	return &PublicModule{s: s, instance: instance}, nil
}

// ReleaseModule deallocates resources if a module with the given name exists.
func (s *Store) ReleaseModule(moduleName string) error {
	m := s.module(moduleName)
	if m == nil {
		return nil // already released
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	if m.dependentCount > 0 {
		// This case other modules are importing this module instance and still alive.
		return fmt.Errorf("%d modules import this and need to be closed first", m.dependentCount)
	}

	// TODO: check outstanding calls and wait until they exit.

	for mod := range m.dependencies {
		mod.decDependentCount()
	}

	if err := s.releaseFunctions(m.Functions...); err != nil {
		return fmt.Errorf("unable to release function instance: %w", err)
	}

	s.deleteModule(m)
	return nil
}

func (m *ModuleInstance) decDependentCount() {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.dependentCount--
}

func (m *ModuleInstance) incDependentCount() {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.dependentCount++
}

func (s *Store) releaseFunctions(fs ...*FunctionInstance) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	for _, f := range fs {
		if err := s.engine.Release(f); err != nil {
			return err
		}

		// Release reference to the function instance.
		s.functions[f.Index] = nil

		// Append the address so that we can reuse it in order to avoid index space explosion.
		s.releasedFunctionIndex[f.Index] = struct{}{}
	}
	return nil
}

func (s *Store) addFunctions(fs ...*FunctionInstance) {
	s.mux.Lock()
	defer s.mux.Unlock()
	for _, f := range fs {
		var index FunctionIndex
		if len(s.releasedFunctionIndex) > 0 {
			for popped := range s.releasedFunctionIndex {
				index = popped
				break
			}
			s.functions[index] = f
			delete(s.releasedFunctionIndex, index)
		} else {
			index = FunctionIndex(len(s.functions))
			s.functions = append(s.functions, f)
		}
		f.Index = index
	}
}

func (s *Store) deleteModule(m *ModuleInstance) {
	s.mux.Lock()
	defer s.mux.Unlock()
	delete(s.modules, m.Name)
}

func (s *Store) addModule(m *ModuleInstance) {
	// Build the default context for calls to this module.
	m.Ctx = NewModuleContext(s.ctx, s.engine, m)

	s.mux.Lock()
	defer s.mux.Unlock()
	s.modules[m.Name] = m
}

// Module implements wasm.Store Module
func (s *Store) Module(moduleName string) publicwasm.Module {
	if m := s.module(moduleName); m != nil {
		return &PublicModule{s: s, instance: m}
	} else {
		return nil
	}
}

func (s *Store) module(moduleName string) *ModuleInstance {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.modules[moduleName]
}

// PublicModule implements wasm.Module
type PublicModule struct {
	s        *Store
	instance *ModuleInstance
}

// String implements fmt.Stringer
func (m *PublicModule) String() string {
	return fmt.Sprintf("Module[%s]", m.instance.Name)
}

// Function implements wasm.Module Function
func (m *PublicModule) Function(name string) publicwasm.Function {
	exp, err := m.instance.getExport(name, ExternTypeFunc)
	if err != nil {
		return nil
	}
	return &exportedFunction{module: m.instance.Ctx, function: exp.Function}
}

// Memory implements wasm.Module Memory
func (m *PublicModule) Memory(name string) publicwasm.Memory {
	exp, err := m.instance.getExport(name, ExternTypeMemory)
	if err != nil {
		return nil
	}
	return exp.Memory
}

// HostModule implements wasm.Store HostModule
func (s *Store) HostModule(moduleName string) publicwasm.HostModule {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.modules[moduleName].hostModule
}

func (s *Store) resolveImports(module *Module) (
	functions []*FunctionInstance, globals []*GlobalInstance,
	table *TableInstance, memory *MemoryInstance,
	moduleImports map[*ModuleInstance]struct{},
	err error,
) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	moduleImports = map[*ModuleInstance]struct{}{}
	for _, is := range module.ImportSection {
		m, ok := s.modules[is.Module]
		if !ok {
			err = fmt.Errorf("module \"%s\" not instantiated", is.Module)
			return
		}

		m.incDependentCount() // TODO: check if the module is already released. See #293
		moduleImports[m] = struct{}{}

		var exp *ExportInstance
		exp, err = m.getExport(is.Name, is.Type)
		if err != nil {
			return
		}

		switch is.Type {
		case ExternTypeFunc:
			typeIndex := is.DescFunc
			if int(typeIndex) >= len(module.TypeSection) {
				err = fmt.Errorf("unknown type for function import")
				return
			}
			expectedType := module.TypeSection[typeIndex]
			f := exp.Function
			if !bytes.Equal(expectedType.Results, f.Type.Results) || !bytes.Equal(expectedType.Params, f.Type.Params) {
				err = fmt.Errorf("signature mimatch: %s != %s", expectedType, f.Type)
				return
			}
			functions = append(functions, f)
		case ExternTypeTable:
			tableType := is.DescTable
			table = exp.Table
			if table.ElemType != tableType.ElemType {
				err = fmt.Errorf("incompatible table import: element type mismatch")
				return
			}
			if table.Min < tableType.Limit.Min {
				err = fmt.Errorf("incompatible table import: minimum size mismatch")
				return
			}

			if tableType.Limit.Max != nil {
				if table.Max == nil {
					err = fmt.Errorf("incompatible table import: maximum size mismatch")
					return
				} else if *table.Max > *tableType.Limit.Max {
					err = fmt.Errorf("incompatible table import: maximum size mismatch")
					return
				}
			}
		case ExternTypeMemory:
			memoryType := is.DescMem
			memory = exp.Memory
			if memory.Min < memoryType.Min {
				err = fmt.Errorf("incompatible memory import: minimum size mismatch")
				return
			}
			if memoryType.Max != nil {
				if memory.Max == nil {
					err = fmt.Errorf("incompatible memory import: maximum size mismatch")
					return
				} else if *memory.Max > *memoryType.Max {
					err = fmt.Errorf("incompatible memory import: maximum size mismatch")
					return
				}
			}
		case ExternTypeGlobal:
			globalType := is.DescGlobal
			g := exp.Global
			if globalType.Mutable != g.Type.Mutable {
				err = fmt.Errorf("incompatible global import: mutability mismatch")
				return
			} else if globalType.ValType != g.Type.ValType {
				err = fmt.Errorf("incompatible global import: value type mismatch")
				return
			}
			globals = append(globals, g)
		}
	}
	return
}

func executeConstExpression(globals []*GlobalInstance, expr *ConstantExpression) (v interface{}) {
	r := bytes.NewReader(expr.Data)
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

func (s *Store) getTypes(ts []*FunctionType) ([]*TypeInstance, error) {
	// We take write-lock here as the follwing might end up mutating typeIDs map.
	s.mux.Lock()
	defer s.mux.Unlock()
	ret := make([]*TypeInstance, len(ts))
	for i, t := range ts {
		inst, err := s.getTypeInstance(t)
		if err != nil {
			return nil, err
		}
		ret[i] = inst
	}
	return ret, nil
}

func (s *Store) getTypeInstance(t *FunctionType) (*TypeInstance, error) {
	key := t.String()
	id, ok := s.typeIDs[key]
	if !ok {
		l := len(s.typeIDs)
		if l >= s.maximumFunctionTypes {
			return nil, fmt.Errorf("too many function types in a store")
		}
		id = FunctionTypeID(len(s.typeIDs))
		s.typeIDs[key] = id
	}
	return &TypeInstance{Type: t, TypeID: id}, nil
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
