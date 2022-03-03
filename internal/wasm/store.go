package internalwasm

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"

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

		// ModuleInstances holds the instantiated Wasm modules by module name from Instantiate.
		moduleInstances map[string]*ModuleInstance

		// TypeIDs maps each FunctionType.String() to a unique FunctionTypeID. This is used at runtime to
		// do type-checks on indirect function calls.
		typeIDs map[string]FunctionTypeID

		// maximumFunctionIndex represents the limit on the number of function addresses (= function instances) in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionIndex FunctionIndex
		//  maximumFunctionTypes represents the limit on the number of function types in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionTypes int

		// releasedFunctionIndex holds reusable FunctionIndexes. An index is added when
		// an function instance is released in releaseFunctionInstances, and is popped when
		// an new instance is added in store.addFunctionInstances.
		releasedFunctionIndex []FunctionIndex

		// releasedMemoryIndex holds reusable memoryIndexes. An index is added when
		// an memory instance is released in releaseMemoryInstance, and is popped when
		// an new instance is added in store.addMemoryInstance.
		releasedMemoryIndex []memoryIndex

		// releasedTableIndex holds reusable tableIndexes. An index is added when
		// an table instance is released in releaseTableInstance, and is popped when
		// an new instance is added in store.addTableInstance.
		releasedTableIndex []tableIndex

		// releasedGlobalIndex holds reusable globalIndexes. An index is added when
		// an global instance is released in releaseGlobalInstances, and is popped when
		// an new instance is added in store.addGlobalInstances.
		releasedGlobalIndex []globalIndex

		// The followings fields match the definition of Store in the specification.

		// Functions holds function instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as funcaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr).
		functions []*FunctionInstance
		// Globals holds global instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#global-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as globaladdr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-globaladdr).
		globals []*GlobalInstance
		// Memories holds memory instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as memaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-memaddr).
		memories []*MemoryInstance
		// Tables holds table instances (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#table-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as tableaddr (https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-tableaddr).
		tables []*TableInstance
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
		// MemoryInstance is set when Module.MemorySection had a memory, regardless of whether it was exported.
		// Note: This avoids the name "Memory" which is an interface method name.
		MemoryInstance *MemoryInstance
		TableInstance  *TableInstance
		Types          []*TypeInstance

		// Ctx holds default function call context from this function instance.
		Ctx *ModuleContext

		// hostModule holds HostModule if this is a "host module" which is created in store.NewHostModule.
		hostModule *HostModule

		// TODO per https://github.com/tetratelabs/wazero/issues/293
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
	// and the index to Store.Functions.
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

// The wazero specific limitations described at RATIONALE.md.
const (
	maximumFunctionIndex = 1 << 27
	maximumFunctionTypes = 1 << 27
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

func NewStore(ctx context.Context, engine Engine, enabledFeatures Features) *Store {
	return &Store{
		ctx:                  ctx,
		engine:               engine,
		EnabledFeatures:      enabledFeatures,
		moduleInstances:      map[string]*ModuleInstance{},
		typeIDs:              map[string]FunctionTypeID{},
		maximumFunctionIndex: maximumFunctionIndex,
		maximumFunctionTypes: maximumFunctionTypes,
	}
}

// checkFunctionIndexOverflow checks if there would be too many function instances in a store.
func (s *Store) checkFunctionIndexOverflow(newInstanceNum int) error {
	if len(s.functions) > int(s.maximumFunctionIndex)-newInstanceNum {
		return fmt.Errorf("too many functions in a store")
	}
	return nil
}

func (s *Store) Instantiate(module *Module, name string) (*PublicModule, error) {
	if err := s.requireModuleUnused(name); err != nil {
		return nil, err
	}

	if err := s.checkFunctionIndexOverflow(len(module.FunctionSection)); err != nil {
		return nil, err
	}

	importedFunctions, importedGlobals, importedTable, importedMemory, moduleImports, err := s.resolveImports(module)
	if err != nil {
		return nil, err
	}

	functions, globals, table, memory :=
		module.buildFunctionInstances(), module.buildGlobalInstances(importedGlobals), module.buildTableInstance(), module.buildMemoryInstance()

	types, err := s.getTypeInstances(module.TypeSection)
	if err != nil {
		return nil, err
	}

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
	s.addFunctionInstances(functions...) // Need to assign funcaddr to each instance before compilation.
	for i, f := range functions {
		if err := s.engine.Compile(f); err != nil {
			// On the failure, release the assigned funcaddr and already compiled functions.
			if err := s.releaseFunctionInstances(functions...); err != nil {
				return nil, err
			}
			idx := module.SectionElementCount(SectionIDFunction) - 1
			return nil, fmt.Errorf("compilation failed at index %d/%d: %w", i, idx, err)
		}
	}

	// Now all the validation passes, we are safe to mutate memory/table instances (possibly imported ones).
	instance.applyElements(module.ElementSection)
	instance.applyData(module.DataSection)

	// Persist the instances other than functions (which we already persisted before compilation).
	s.addGlobalInstances(globals...)
	s.addTableInstance(table)
	s.addMemoryInstance(instance.MemoryInstance)

	// Increase the reference count of imported modules.
	for imported := range instance.moduleImports {
		imported.refCount++
	}

	// Build the default context for calls to this module.
	instance.Ctx = NewModuleContext(s.ctx, s.engine, instance)

	s.moduleInstances[instance.Name] = instance

	// Execute the start function.
	if module.StartSection != nil {
		funcIdx := *module.StartSection
		if _, err := s.engine.Call(instance.Ctx, instance.Functions[funcIdx]); err != nil {
			return nil, fmt.Errorf("module[%s] start function failed: %w", name, err)
		}
	}
	return &PublicModule{s, instance}, nil
}

func (s *Store) ReleaseModuleInstance(instance *ModuleInstance) error {
	instance.refCount--
	if instance.refCount > 0 {
		// This case other modules are importing this module instance and still alive.
		return nil
	}

	// TODO: check outstanding calls and wait until they exit.

	// Recursively release the imported instances.
	for mod := range instance.moduleImports {
		if err := s.ReleaseModuleInstance(mod); err != nil {
			return fmt.Errorf("unable to release imported module [%s]: %w", mod.Name, err)
		}
	}

	if err := s.releaseFunctionInstances(instance.Functions...); err != nil {
		return fmt.Errorf("unable to release function instance: %w", err)
	}
	s.releaseMemoryInstance(instance.MemoryInstance)
	s.releaseTableInstance(instance.TableInstance)
	s.releaseGlobalInstances(instance.Globals...)

	// Explicitly assign nil so that we ensure this moduleInstance no longer holds reference to instances.
	instance.Exports = nil
	instance.Globals = nil
	instance.Functions = nil
	instance.TableInstance = nil
	instance.MemoryInstance = nil
	instance.Types = nil

	delete(s.moduleInstances, instance.Name)
	return nil
}

func (s *Store) releaseFunctionInstances(fs ...*FunctionInstance) error {
	for _, f := range fs {
		if err := s.engine.Release(f); err != nil {
			return err
		}

		// Release refernce to the function instance.
		s.functions[f.Index] = nil

		// Append the address so that we can reuse it in order to avoid index space explosion.
		s.releasedFunctionIndex = append(s.releasedFunctionIndex, f.Index)
	}
	return nil
}

func (s *Store) addFunctionInstances(fs ...*FunctionInstance) {
	for _, f := range fs {
		var addr FunctionIndex
		if len(s.releasedFunctionIndex) > 0 {
			id := len(s.releasedFunctionIndex) - 1
			// Pop one address from releasedFunctionIndex slice.
			addr, s.releasedFunctionIndex = s.releasedFunctionIndex[id], s.releasedFunctionIndex[:id]
			s.functions[f.Index] = f
		} else {
			addr = FunctionIndex(len(s.functions))
			s.functions = append(s.functions, f)
		}
		f.Index = addr
	}
}

func (s *Store) releaseGlobalInstances(gs ...*GlobalInstance) {
	for _, g := range gs {
		// Release reference to the global instance.
		s.globals[g.index] = nil

		// Append the address so that we can reuse it in order to avoid index space explosion.
		s.releasedGlobalIndex = append(s.releasedGlobalIndex, g.index)
	}
}

func (s *Store) addGlobalInstances(gs ...*GlobalInstance) {
	for _, g := range gs {
		var addr globalIndex
		if len(s.releasedGlobalIndex) > 0 {
			id := len(s.releasedGlobalIndex) - 1
			// Pop one address from releasedGlobalIndex slice.
			addr, s.releasedGlobalIndex = s.releasedGlobalIndex[id], s.releasedGlobalIndex[:id]
			s.globals[g.index] = g
		} else {
			addr = globalIndex(len(s.globals))
			s.globals = append(s.globals, g)
		}
		g.index = addr
	}
}

func (s *Store) releaseTableInstance(t *TableInstance) {
	// Release refernce to the table instance.
	s.tables[t.index] = nil

	// Append the index so that we can reuse it in order to avoid index space explosion.
	s.releasedTableIndex = append(s.releasedTableIndex, t.index)
}

func (s *Store) addTableInstance(t *TableInstance) {
	if t == nil {
		return
	}

	var addr tableIndex
	if len(s.releasedTableIndex) > 0 {
		id := len(s.releasedTableIndex) - 1
		// Pop one index from releasedTableIndex slice.
		addr, s.releasedTableIndex = s.releasedTableIndex[id], s.releasedTableIndex[:id]
		s.tables[addr] = t
	} else {
		addr = tableIndex(len(s.tables))
		s.tables = append(s.tables, t)
	}
	t.index = addr
}

func (s *Store) releaseMemoryInstance(m *MemoryInstance) {
	// Release refernce to the memory instance.
	s.memories[m.index] = nil

	// Append the index so that we can reuse it in order to avoid index space explosion.
	s.releasedMemoryIndex = append(s.releasedMemoryIndex, m.index)
}

func (s *Store) addMemoryInstance(m *MemoryInstance) {
	if m == nil {
		return
	}

	var addr memoryIndex
	if len(s.releasedMemoryIndex) > 0 {
		id := len(s.releasedMemoryIndex) - 1
		// Pop one index from releasedMemoryIndex slice.
		addr, s.releasedMemoryIndex = s.releasedMemoryIndex[id], s.releasedMemoryIndex[:id]
		s.memories[addr] = m
	} else {
		addr = memoryIndex(len(s.memories))
		s.memories = append(s.memories, m)
	}
	m.index = addr
}

// Module implements wasm.Store Module
func (s *Store) Module(moduleName string) publicwasm.Module {
	if m, ok := s.moduleInstances[moduleName]; !ok {
		return nil
	} else {
		return &PublicModule{s, m}
	}
}

// PublicModule implements wasm.Module
type PublicModule struct {
	s *Store
	// Context is exported for /wasi.go
	Instance *ModuleInstance
}

// Function implements wasm.Module Function
func (m *PublicModule) Function(name string) publicwasm.Function {
	exp, err := m.Instance.GetExport(name, ExternTypeFunc)
	if err != nil {
		return nil
	}
	return &exportedFunction{module: m.Instance.Ctx, function: exp.Function}
}

// Memory implements wasm.Module Memory
func (m *PublicModule) Memory(name string) publicwasm.Memory {
	exp, err := m.Instance.GetExport(name, ExternTypeMemory)
	if err != nil {
		return nil
	}
	return exp.Memory
}

// HostModule implements wasm.Store HostModule
func (s *Store) HostModule(moduleName string) publicwasm.HostModule {
	return s.moduleInstances[moduleName].hostModule
}

func (s *Store) getExport(moduleName string, name string, et ExternType) (exp *ExportInstance, err error) {
	if m, ok := s.moduleInstances[moduleName]; !ok {
		return nil, fmt.Errorf("module %s not instantiated", moduleName)
	} else if exp, err = m.GetExport(name, et); err != nil {
		return
	}
	return
}

func (s *Store) resolveImports(module *Module) (
	functions []*FunctionInstance, globals []*GlobalInstance,
	table *TableInstance, memory *MemoryInstance,
	moduleImports map[*ModuleInstance]struct{},
	err error,
) {
	moduleImports = map[*ModuleInstance]struct{}{}
	for _, is := range module.ImportSection {
		m, ok := s.moduleInstances[is.Module]
		if !ok {
			err = fmt.Errorf("module \"%s\" not instantiated", is.Module)
			return
		}

		// Note: at this point we don't increase the ref count.
		moduleImports[m] = struct{}{}

		var exp *ExportInstance
		exp, err = m.GetExport(is.Name, is.Type)
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
			if !bytes.Equal(expectedType.Results, f.FunctionType.Type.Results) || !bytes.Equal(expectedType.Params, f.FunctionType.Type.Params) {
				err = fmt.Errorf("signature mimatch: %s != %s", expectedType, f.FunctionType.Type)
				return
			}
			functions = append(functions, f)
		case ExternTypeTable:
			tableType := is.DescTable
			table = exp.Table
			if table.ElemType != tableType.ElemType {
				err = fmt.Errorf("incompatible table improt: element type mismatch")
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

// Only used in spectests.
func (s *Store) AliasModuleInstance(src, dst string) {
	s.moduleInstances[dst] = s.moduleInstances[src]
}

func (s *Store) getTypeInstances(ts []*FunctionType) ([]*TypeInstance, error) {
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
	// TODO: take mutex
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
