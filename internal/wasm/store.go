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

		// moduleNames ensures no race conditions instantiating two modules of the same name
		moduleNames map[string]struct{} // guarded by mux

		// modules holds the instantiated Wasm modules by module name from Instantiate.
		modules map[string]*ModuleInstance // guarded by mux

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
		// which are only owned by ModuleInstance. This is not only because the function call implementation in engines depend on
		// store-scope funcaddr (FunctionIndex in wazero), but also call_indirect is specified to make function calls via funcaddr,
		// which might end up calling a function outside of module-scope index space.
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

// addSections adds section elements to the ModuleInstance
func (m *ModuleInstance) addSections(module *Module, importedFunctions, functions []*FunctionInstance,
	importedGlobals, globals []*GlobalInstance, importedTable, table *TableInstance,
	memory, importedMemory *MemoryInstance, typeInstances []*TypeInstance, moduleImports map[*ModuleInstance]struct{}) {

	m.Types = typeInstances
	m.dependencies = moduleImports

	m.Functions = append(m.Functions, importedFunctions...)
	for i, f := range functions {
		// Associate each function with the type instance and the module instance's pointer.
		f.Module = m
		f.TypeID = typeInstances[module.FunctionSection[i]].TypeID
		m.Functions = append(m.Functions, f)
	}

	m.Globals = append(m.Globals, importedGlobals...)
	m.Globals = append(m.Globals, globals...)

	if importedTable != nil {
		m.Table = importedTable
	} else {
		m.Table = table
	}

	if importedMemory != nil {
		m.Memory = importedMemory
	} else {
		m.Memory = memory
	}

	m.buildExports(module.ExportSection)
}

func (m *ModuleInstance) buildExports(exports map[string]*Export) {
	m.Exports = make(map[string]*ExportInstance, len(exports))
	for _, exp := range exports {
		index := exp.Index
		var ei *ExportInstance
		switch exp.Type {
		case ExternTypeFunc:
			ei = &ExportInstance{Type: exp.Type, Function: m.Functions[index]}
			// The module instance of the host function is a fake that only includes the function and its types.
			// We need to assign the ModuleInstance when re-exporting so that any memory defined in the target is
			// available to the wasm.Module Memory.
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
		moduleNames:           map[string]struct{}{},
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

// Instantiate uses name instead of the Module.NameSection ModuleName as it allows instantiating the same module under
// different names safely and concurrently.
func (s *Store) Instantiate(module *Module, name string) (*ModuleContext, error) {
	if err := s.requireModuleName(name); err != nil {
		return nil, err
	}

	// Note: we do not take lock here in order to enable concurrent instantiation and compilation
	// of multiple modules. When necessary, we take read or write locks in each method of store used here.
	if err := s.checkFunctionIndexOverflow(len(module.FunctionSection)); err != nil {
		s.deleteModule(name)
		return nil, err
	}

	types, err := s.getTypes(module.TypeSection)
	if err != nil {
		s.deleteModule(name)
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
		s.deleteModule(name)
		return nil, err
	}

	globals, table, memory := module.buildGlobals(importedGlobals), module.buildTable(), module.buildMemory()

	// If there are no module-defined functions, assume this is a host module.
	var functions []*FunctionInstance
	var funcSection SectionID
	if module.HostFunctionSection == nil {
		funcSection = SectionIDFunction
		functions = module.buildFunctions()
	} else {
		funcSection = SectionIDHostFunction
		functions = module.buildHostFunctionInstances()
	}

	// Now we have all instances from imports and local ones, so ready to create a new ModuleInstance.
	m := &ModuleInstance{Name: name}
	m.addSections(module, importedFunctions, functions, importedGlobals,
		globals, importedTable, table, importedMemory, memory, types, moduleImports)

	if err = m.validateElements(module.ElementSection); err != nil {
		s.deleteModule(name)
		return nil, err
	}

	if err = m.validateData(module.DataSection); err != nil {
		s.deleteModule(name)
		return nil, err
	}

	// Now we are ready to compile functions.
	s.addFunctions(functions...) // Need to assign funcaddr to each instance before compilation.
	for i, f := range functions {
		// TODO: maybe better consider spawning multiple goroutines for compilations to accelerate.
		if err = s.engine.Compile(f); err != nil {
			s.deleteModule(name)
			// On the failure, release the assigned funcaddr and already compiled functions.
			_ = s.releaseFunctions(functions[:i]...) // ignore any release error, so we can report the original one.
			return nil, fmt.Errorf("%s compilation failed: %w", module.funcDesc(funcSection, Index(i)), err)
		}
	}

	// Now all the validation passes, we are safe to mutate memory/table instances (possibly imported ones).
	m.applyElements(module.ElementSection)
	m.applyData(module.DataSection)

	// Build the default context for calls to this module.
	m.Ctx = NewModuleContext(s.ctx, s.engine, m)

	// Plus, we can finalize the module import reference count.
	moduleImportsFinalized = true

	// Execute the start function.
	if module.StartSection != nil {
		funcIdx := *module.StartSection
		if _, err = s.engine.Call(m.Ctx, m.Functions[funcIdx]); err != nil {
			s.deleteModule(name)
			return nil, fmt.Errorf("start %s failed: %w", module.funcDesc(funcSection, funcIdx), err)
		}
	}

	// Now that the instantiation is complete without error, add it. This makes it visible for import.
	s.addModule(m)
	return m.Ctx, nil
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

	s.deleteModule(moduleName)
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

// deleteModule makes the moduleName available for instantiation again.
func (s *Store) deleteModule(moduleName string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	delete(s.modules, moduleName)
	delete(s.moduleNames, moduleName)
}

// requireModuleName is a pre-flight check to reserve a module.
// This must be reverted on error with deleteModule if initialization fails.
func (s *Store) requireModuleName(moduleName string) error {
	s.mux.Lock()
	defer s.mux.Unlock()
	if _, ok := s.moduleNames[moduleName]; ok {
		return fmt.Errorf("module %s has already been instantiated", moduleName)
	}
	s.moduleNames[moduleName] = struct{}{}
	return nil
}

// addModule makes the module visible for import
func (s *Store) addModule(m *ModuleInstance) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.modules[m.Name] = m
}

// Module implements wazero.Runtime Module
func (s *Store) Module(moduleName string) publicwasm.Module {
	if m := s.module(moduleName); m != nil {
		return m.Ctx
	} else {
		return nil
	}
}

func (s *Store) module(moduleName string) *ModuleInstance {
	s.mux.RLock()
	defer s.mux.RUnlock()
	return s.modules[moduleName]
}

func (s *Store) resolveImports(module *Module) (
	importedFunctions []*FunctionInstance, importedGlobals []*GlobalInstance,
	importedTable *TableInstance, importedMemory *MemoryInstance,
	moduleImports map[*ModuleInstance]struct{},
	err error,
) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	moduleImports = map[*ModuleInstance]struct{}{}
	for idx, i := range module.ImportSection {
		m, ok := s.modules[i.Module]
		if !ok {
			err = fmt.Errorf("module[%s] not instantiated", i.Module)
			return
		}

		m.incDependentCount() // TODO: check if the module is already released. See #293
		moduleImports[m] = struct{}{}

		var imported *ExportInstance
		imported, err = m.getExport(i.Name, i.Type)
		if err != nil {
			return
		}

		switch i.Type {
		case ExternTypeFunc:
			typeIndex := i.DescFunc
			// TODO: this shouldn't be possible as invalid should fail validate
			if int(typeIndex) >= len(module.TypeSection) {
				err = errorInvalidImport(i, idx, fmt.Errorf("function type out of range"))
				return
			}
			expectedType := module.TypeSection[i.DescFunc]
			importedFunction := imported.Function

			actualType := importedFunction.Type
			if !expectedType.EqualsSignature(actualType.Params, actualType.Results) {
				err = errorInvalidImport(i, idx, fmt.Errorf("signature mismatch: %s != %s", expectedType, actualType))
				return
			}

			importedFunctions = append(importedFunctions, importedFunction)
		case ExternTypeTable:
			expected := i.DescTable
			importedTable = imported.Table

			if importedTable.ElemType != expected.ElemType {
				err = errorInvalidImport(i, idx, fmt.Errorf("element type mismatch: %s != %s",
					ValueTypeName(expected.ElemType), ValueTypeName(importedTable.ElemType)))
				return
			}

			if expected.Limit.Min > importedTable.Min {
				err = errorMinSizeMismatch(i, idx, expected.Limit.Min, importedTable.Min)
				return
			}

			if expected.Limit.Max != nil {
				expectedMax := *expected.Limit.Max
				if importedTable.Max == nil {
					err = errorNoMax(i, idx, expectedMax)
					return
				} else if expectedMax < *importedTable.Max {
					err = errorMaxSizeMismatch(i, idx, expectedMax, *importedTable.Max)
					return
				}
			}
		case ExternTypeMemory:
			expected := i.DescMem
			importedMemory = imported.Memory

			if expected.Min > importedMemory.Min {
				err = errorMinSizeMismatch(i, idx, expected.Min, importedMemory.Min)
				return
			}

			if expected.Max != nil {
				expectedMax := *expected.Max
				if importedMemory.Max == nil {
					err = errorNoMax(i, idx, expectedMax)
					return
				} else if expectedMax < *importedMemory.Max {
					err = errorMaxSizeMismatch(i, idx, expectedMax, *importedMemory.Max)
					return
				}
			}
		case ExternTypeGlobal:
			expected := i.DescGlobal
			importedGlobal := imported.Global

			if expected.Mutable != importedGlobal.Type.Mutable {
				err = errorInvalidImport(i, idx, fmt.Errorf("mutability mismatch: %t != %t",
					expected.Mutable, importedGlobal.Type.Mutable))
				return
			}

			if expected.ValType != importedGlobal.Type.ValType {
				err = errorInvalidImport(i, idx, fmt.Errorf("value type mismatch: %s != %s",
					ValueTypeName(expected.ValType), ValueTypeName(importedGlobal.Type.ValType)))
				return
			}
			importedGlobals = append(importedGlobals, importedGlobal)
		}
	}
	return
}

func errorMinSizeMismatch(i *Import, idx int, expected, actual uint32) error {
	return errorInvalidImport(i, idx, fmt.Errorf("minimum size mismatch: %d > %d", expected, actual))
}

func errorNoMax(i *Import, idx int, expected uint32) error {
	return errorInvalidImport(i, idx, fmt.Errorf("maximum size mismatch: %d, but actual has no max", expected))
}

func errorMaxSizeMismatch(i *Import, idx int, expected, actual uint32) error {
	return errorInvalidImport(i, idx, fmt.Errorf("maximum size mismatch: %d < %d", expected, actual))
}

func errorInvalidImport(i *Import, idx int, err error) error {
	return fmt.Errorf("import[%d] %s[%s.%s]: %w", idx, ExternTypeName(i.Type), i.Module, i.Name, err)
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
