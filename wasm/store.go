package wasm

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/tetratelabs/wazero/wasm/internal/ieee754"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
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
	// See https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0
	Store struct {
		// The following fields are wazero-specific fields of Store.

		// engine is a global context for a Store which is in reponsible for compilation and execution of Wasm modules.
		engine Engine
		// ModuleInstances holds the instantiated Wasm modules keyed on names given at Instantiate.
		ModuleInstances map[string]*ModuleInstance
		// TypeIDs maps each FunctionType.String() to a unique FunctionTypeID. This is used at runtime to
		// do type-checks on indirect function calls.
		TypeIDs map[string]FunctionTypeID

		// maximumFunctionAddress and maximumFunctionTypes represent the limit on the number of each instance type in a store.
		maximumFunctionAddress, maximumFunctionTypes int

		// The followings fields match the definition of Store in the specification.

		// Functions holds function instances (https://www.w3.org/TR/wasm-core-1/#function-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as funcaddr (https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr).
		Functions []*FunctionInstance
		// Globals holds global instances (https://www.w3.org/TR/wasm-core-1/#global-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as globaladdr (https://www.w3.org/TR/wasm-core-1/#syntax-globaladdr).
		Globals []*GlobalInstance
		// Memories holds memory instances (https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as memaddr (https://www.w3.org/TR/wasm-core-1/#syntax-memaddr).
		Memories []*MemoryInstance
		// Tables holds table instances (https://www.w3.org/TR/wasm-core-1/#table-instances%E2%91%A0),
		// in this store.
		// The slice index is to be interpreted as tableaddr (https://www.w3.org/TR/wasm-core-1/#syntax-tableaddr).
		Tables []*TableInstance
	}

	// ModuleInstance represents instantiated wasm module.
	// The difference from the spec is that in wazero, a ModuleInstance holds pointers
	// to the instances, rather than "addresses" (i.e. index to Store.Functions, Globals, etc) for convenience.
	//
	// See https://www.w3.org/TR/wasm-core-1/#syntax-moduleinst
	ModuleInstance struct {
		Name      string
		Exports   map[string]*ExportInstance
		Functions []*FunctionInstance
		Globals   []*GlobalInstance
		Memory    *MemoryInstance
		Tables    []*TableInstance
		Types     []*TypeInstance
	}

	// ExportInstance represents an exported instance in a Store.
	// The difference from the spec is that in wazero, a ExportInstance holds pointers
	// to the instances, rather than "addresses" (i.e. index to Store.Functions, Globals, etc) for convenience.
	//
	// See https://www.w3.org/TR/wasm-core-1/#syntax-exportinst
	ExportInstance struct {
		Kind     ExportKind
		Function *FunctionInstance
		Global   *GlobalInstance
		Memory   *MemoryInstance
		Table    *TableInstance
	}

	// FunctionInstance represents a function instance in a Store.
	// See https://www.w3.org/TR/wasm-core-1/#function-instances%E2%91%A0
	FunctionInstance struct {
		// ModuleInstance holds the pointer to the module instance to which this function belongs.
		ModuleInstance *ModuleInstance
		// Body is the function body in WebAssembly Binary Format
		Body []byte
		// FunctionType holds the pointer to TypeInstance whose functionType field equals that of this function.
		FunctionType *TypeInstance
		// LocalTypes holds types of locals.
		LocalTypes []ValueType
		// HostFunction holds the runtime representation of host functions.
		// If this is not nil, all the above fields are ignored as they are specific to non-host functions.
		HostFunction *reflect.Value
		// Address is the funcaddr(https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr) of this function insntance.
		// More precisely, this equals the index of this function instance in store.FunctionInstances.
		// All function calls are made via funcaddr at runtime, not the index (scoped to a module).
		//
		// This is used by both host and non-host functions.
		Address FunctionAddress
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
	// See https://www.w3.org/TR/wasm-core-1/#global-instances%E2%91%A0
	GlobalInstance struct {
		Type *GlobalType
		// Val holds a 64-bit representation of the actual value.
		Val uint64
	}

	// TableInstance represents a table instance in a store.
	// See https://www.w3.org/TR/wasm-core-1/#table-instances%E2%91%A0
	//
	// Note this is fixed to function type until post MVP reference type is implemented.
	TableInstance struct {
		// Table holds the table elements managed by this table instance.
		//
		// Note: we intentionally use "[]TableElement", not "[]*TableElement",
		// because the JIT engine accesses this slice directly from assembly.
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
	// Note: this is fixed to function type as it is the only supported type in WebAssembly 1.0 (MVP)
	TableElement struct {
		// FunctionAddress is funcaddr (https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr)
		// of the target function instance. More precisely, this equals the index of
		// the target function instance in Store.FunctionInstances.
		FunctionAddress FunctionAddress
		// FunctionTypeID is the type ID of the target function's type, which
		// equals store.Functions[FunctionAddress].FunctionType.TypeID.
		FunctionTypeID FunctionTypeID
	}

	// MemoryInstance represents a memory instance in a store.
	// See https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0.
	MemoryInstance struct {
		Buffer []byte
		Min    uint32
		Max    *uint32
	}

	// FunctionAddress is funcaddr (https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr),
	// and the index to Store.Functions.
	FunctionAddress uint64

	// FunctionTypeID is an uniquely assigned integer for a function type.
	// This is wazero specific runtime object and specific to a store,
	// and used at runtime to do type-checks on indirect function calls.
	FunctionTypeID uint32
)

const (
	maximumFunctionAddress = math.MaxUint32
	maximumFunctionTypes   = maximumFunctionAddress
)

// addExport adds and indexes the given export or errs if the name is already exported.
func (m *ModuleInstance) addExport(name string, e *ExportInstance) error {
	if _, ok := m.Exports[name]; ok {
		return fmt.Errorf("%q is already exported in module %q", name, m.Name)
	}
	m.Exports[name] = e
	return nil
}

// GetExport returns an export of the given name and kind or errs if not exported or the wrong kind.
func (m *ModuleInstance) GetExport(name string, kind ExportKind) (*ExportInstance, error) {
	exp, ok := m.Exports[name]
	if !ok {
		return nil, fmt.Errorf("%q is not exported in module %q", name, m.Name)
	}
	if exp.Kind != kind {
		return nil, fmt.Errorf("export %q in module %q is a %s, not a %s", name, m.Name, exportKindName(exp.Kind), exportKindName(kind))
	}
	return exp, nil
}

func (f *FunctionInstance) IsHostFunction() bool {
	return f.HostFunction != nil
}

func NewStore(engine Engine) *Store {
	return &Store{
		ModuleInstances:        map[string]*ModuleInstance{},
		TypeIDs:                map[string]FunctionTypeID{},
		engine:                 engine,
		maximumFunctionAddress: maximumFunctionAddress,
		maximumFunctionTypes:   maximumFunctionTypes,
	}
}

func (s *Store) Instantiate(module *Module, name string) error {
	instance := &ModuleInstance{Name: name}
	for _, t := range module.TypeSection {
		typeInstance, err := s.getTypeInstance(t)
		if err != nil {
			return err
		}
		instance.Types = append(instance.Types, typeInstance)
	}

	s.ModuleInstances[name] = instance
	// Resolve the imports before doing the actual instantiation (mutating store).
	if err := s.resolveImports(module, instance); err != nil {
		return fmt.Errorf("resolve imports: %w", err)
	}
	// Instantiation.
	// Note that some of them mutate the store, so
	// in the case of errors, we must rollback the state of store.
	var rollbackFuncs []func()
	defer func() {
		for _, f := range rollbackFuncs {
			f()
		}
	}()
	rs, err := s.buildGlobalInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return fmt.Errorf("globals: %w", err)
	}
	rs, err = s.buildFunctionInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return fmt.Errorf("functions: %w", err)
	}
	rs, err = s.buildTableInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return fmt.Errorf("tables: %w", err)
	}
	rs, err = s.buildMemoryInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return fmt.Errorf("memories: %w", err)
	}
	rs, err = s.buildExportInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return fmt.Errorf("exports: %w", err)
	}

	for i, f := range instance.Functions {
		if err := s.engine.Compile(f); err != nil {
			return fmt.Errorf("compilation failed at index %d/%d: %v", i, len(module.FunctionSection)-1, err)
		}
	}

	// Check the start function is valid.
	if startIndex := module.StartSection; startIndex != nil {
		index := *startIndex
		if int(index) >= len(instance.Functions) {
			return fmt.Errorf("invalid start function index: %d", index)
		}
		ft := instance.Functions[index].FunctionType
		if len(ft.Type.Params) != 0 || len(ft.Type.Results) != 0 {
			return fmt.Errorf("start function must have the empty function type")
		}
	}

	// Now we are safe to finalize the state.
	rollbackFuncs = nil

	// Execute the start function.
	if startIndex := module.StartSection; startIndex != nil {
		f := instance.Functions[*startIndex]
		if _, err := s.engine.Call(f); err != nil {
			return fmt.Errorf("calling start function failed: %v", err)
		}
	}
	return nil
}

func (s *Store) CallFunction(moduleName, funcName string, params ...uint64) (results []uint64, resultTypes []ValueType, err error) {
	var exp *ExportInstance
	if exp, err = s.getExport(moduleName, funcName, ExportKindFunc); err != nil {
		return
	}

	f := exp.Function
	if len(f.FunctionType.Type.Params) != len(params) {
		err = fmt.Errorf("invalid number of parameters")
		return
	}

	results, err = s.engine.Call(f, params...)
	resultTypes = f.FunctionType.Type.Results
	return
}

func (s *Store) getExport(moduleName string, name string, kind ExportKind) (exp *ExportInstance, err error) {
	if m, ok := s.ModuleInstances[moduleName]; !ok {
		return nil, fmt.Errorf("module %s not instantiated", moduleName)
	} else if exp, err = m.GetExport(name, kind); err != nil {
		return
	}
	return
}

func (s *Store) addFunctionInstance(f *FunctionInstance) error {
	l := len(s.Functions)
	if l >= s.maximumFunctionAddress {
		return fmt.Errorf("too many functions in a store")
	}
	f.Address = FunctionAddress(len(s.Functions))
	s.Functions = append(s.Functions, f)
	return nil
}

func (s *Store) resolveImports(module *Module, target *ModuleInstance) error {
	for _, is := range module.ImportSection {
		if err := s.resolveImport(target, is); err != nil {
			return fmt.Errorf("%s: %w", is.Name, err)
		}
	}
	return nil
}

func (s *Store) resolveImport(target *ModuleInstance, is *Import) error {
	exp, err := s.getExport(is.Module, is.Name, is.Kind)
	if err != nil {
		return err
	}

	switch is.Kind {
	case ImportKindFunc:
		if err = s.applyFunctionImport(target, is.DescFunc, exp); err != nil {
			return fmt.Errorf("applyFunctionImport: %w", err)
		}
	case ImportKindTable:
		if err = s.applyTableImport(target, is.DescTable, exp); err != nil {
			return fmt.Errorf("applyTableImport: %w", err)
		}
	case ImportKindMemory:
		if err = s.applyMemoryImport(target, is.DescMem, exp); err != nil {
			return fmt.Errorf("applyMemoryImport: %w", err)
		}
	case ImportKindGlobal:
		if err = s.applyGlobalImport(target, is.DescGlobal, exp); err != nil {
			return fmt.Errorf("applyGlobalImport: %w", err)
		}
	default:
		return fmt.Errorf("invalid kind of import: %#x", is.Kind)
	}

	return nil
}

func (s *Store) applyFunctionImport(target *ModuleInstance, typeIndex Index, externModuleExportInstance *ExportInstance) error {
	f := externModuleExportInstance.Function
	if int(typeIndex) >= len(target.Types) {
		return fmt.Errorf("unknown type for function import")
	}
	expectedType := target.Types[typeIndex].Type
	if !bytes.Equal(expectedType.Results, f.FunctionType.Type.Results) {
		return fmt.Errorf("return signature mimatch: %#x != %#x", expectedType.Results, f.FunctionType.Type.Results)
	} else if !bytes.Equal(expectedType.Params, f.FunctionType.Type.Params) {
		return fmt.Errorf("input signature mimatch: %#x != %#x", expectedType.Params, f.FunctionType.Type.Params)
	}
	target.Functions = append(target.Functions, f)
	return nil
}

func (s *Store) applyTableImport(target *ModuleInstance, tableTypePtr *TableType, externModuleExportIsntance *ExportInstance) error {
	table := externModuleExportIsntance.Table
	if tableTypePtr == nil {
		return fmt.Errorf("table type is invalid")
	}
	if table.ElemType != tableTypePtr.ElemType {
		return fmt.Errorf("incompatible table imports: element type mismatch")
	}
	if table.Min < tableTypePtr.Limit.Min {
		return fmt.Errorf("incompatible table imports: minimum size mismatch")
	}

	if tableTypePtr.Limit.Max != nil {
		if table.Max == nil {
			return fmt.Errorf("incompatible table imports: maximum size mismatch")
		} else if *table.Max > *tableTypePtr.Limit.Max {
			return fmt.Errorf("incompatible table imports: maximum size mismatch")
		}
	}
	target.Tables = append(target.Tables, table)
	return nil
}

func (s *Store) applyMemoryImport(target *ModuleInstance, memoryTypePtr *MemoryType, externModuleExportIsntance *ExportInstance) error {
	if target.Memory != nil {
		// The current Wasm spec doesn't allow multiple memories.
		return fmt.Errorf("multiple memories are not supported")
	} else if memoryTypePtr == nil {
		return fmt.Errorf("memory type is invalid")
	}
	memory := externModuleExportIsntance.Memory
	if memory.Min < memoryTypePtr.Min {
		return fmt.Errorf("incompatible memory imports: minimum size mismatch")
	}
	if memoryTypePtr.Max != nil {
		if memory.Max == nil {
			return fmt.Errorf("incompatible memory imports: maximum size mismatch")
		} else if *memory.Max > *memoryTypePtr.Max {
			return fmt.Errorf("incompatible memory imports: maximum size mismatch")
		}
	}
	target.Memory = memory
	return nil
}

func (s *Store) applyGlobalImport(target *ModuleInstance, globalTypePtr *GlobalType, externModuleExportIsntance *ExportInstance) error {
	if globalTypePtr == nil {
		return fmt.Errorf("global type is invalid")
	}
	g := externModuleExportIsntance.Global
	if globalTypePtr.Mutable != g.Type.Mutable {
		return fmt.Errorf("incompatible global import: mutability mismatch")
	} else if globalTypePtr.ValType != g.Type.ValType {
		return fmt.Errorf("incompatible global import: value type mismatch")
	}
	target.Globals = append(target.Globals, g)
	return nil
}

func (s *Store) executeConstExpression(target *ModuleInstance, expr *ConstantExpression) (v interface{}, valueType ValueType, err error) {
	r := bytes.NewBuffer(expr.Data)
	switch expr.Opcode {
	case OpcodeI32Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read uint32: %w", err)
		}
		return v, ValueTypeI32, nil
	case OpcodeI64Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read uint64: %w", err)
		}
		return v, ValueTypeI64, nil
	case OpcodeF32Const:
		v, err = ieee754.DecodeFloat32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read f32: %w", err)
		}
		return v, ValueTypeF32, nil
	case OpcodeF64Const:
		v, err = ieee754.DecodeFloat64(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read f64: %w", err)
		}
		return v, ValueTypeF64, nil
	case OpcodeGlobalGet:
		id, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read index of global: %w", err)
		}
		if uint32(len(target.Globals)) <= id {
			return nil, 0, fmt.Errorf("global index out of range")
		}
		g := target.Globals[id]
		switch g.Type.ValType {
		case ValueTypeI32:
			v = int32(g.Val)
			return v, ValueTypeI32, nil
		case ValueTypeI64:
			v = int64(g.Val)
			return v, ValueTypeI64, nil
		case ValueTypeF32:
			v = math.Float32frombits(uint32(g.Val))
			return v, ValueTypeF32, nil
		case ValueTypeF64:
			v = math.Float64frombits(g.Val)
			return v, ValueTypeF64, nil
		}
	}
	return nil, 0, fmt.Errorf("invalid opt code: %#x", expr.Opcode)
}

func (s *Store) buildGlobalInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	prevLen := len(s.Globals)
	rollbackFuncs = append(rollbackFuncs, func() {
		s.Globals = s.Globals[:prevLen]
	})
	for _, gs := range module.GlobalSection {
		raw, t, err := s.executeConstExpression(target, gs.Init)
		if err != nil {
			return rollbackFuncs, fmt.Errorf("execution failed: %w", err)
		}
		if gs.Type.ValType != t {
			return rollbackFuncs, fmt.Errorf("global type mismatch")
		}
		var gv uint64
		switch v := raw.(type) {
		case int32:
			gv = uint64(v)
		case int64:
			gv = uint64(v)
		case float32:
			gv = uint64(math.Float32bits(v))
		case float64:
			gv = math.Float64bits(v)
		}
		g := &GlobalInstance{
			Type: gs.Type,
			Val:  gv,
		}
		target.Globals = append(target.Globals, g)
		s.Globals = append(s.Globals, g)
	}
	return rollbackFuncs, nil
}

func (s *Store) buildFunctionInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	prevLen := len(s.Functions)
	rollbackFuncs = append(rollbackFuncs, func() {
		s.Functions = s.Functions[:prevLen]
	})
	var functionDeclarations []Index
	var globalDeclarations []*GlobalType
	var memoryDeclarations []*MemoryType
	var tableDeclarations []*TableType
	for _, imp := range module.ImportSection {
		switch imp.Kind {
		case ImportKindFunc:
			functionDeclarations = append(functionDeclarations, imp.DescFunc)
		case ImportKindGlobal:
			globalDeclarations = append(globalDeclarations, imp.DescGlobal)
		case ImportKindMemory:
			memoryDeclarations = append(memoryDeclarations, imp.DescMem)
		case ImportKindTable:
			tableDeclarations = append(tableDeclarations, imp.DescTable)
		}
	}
	importedFunctionCount := len(functionDeclarations)
	functionDeclarations = append(functionDeclarations, module.FunctionSection...)
	for _, g := range module.GlobalSection {
		globalDeclarations = append(globalDeclarations, g.Type)
	}
	memoryDeclarations = append(memoryDeclarations, module.MemorySection...)
	tableDeclarations = append(tableDeclarations, module.TableSection...)

	var functionNames NameMap
	if module.NameSection != nil {
		functionNames = module.NameSection.FunctionNames
	}

	n, nLen := 0, len(functionNames)

	analysisCache := map[int]map[uint64]struct{}{}

	for codeIndex, typeIndex := range module.FunctionSection {
		if typeIndex >= uint32(len(module.TypeSection)) {
			return rollbackFuncs, fmt.Errorf("function type index out of range")
		} else if codeIndex >= len(module.CodeSection) {
			return rollbackFuncs, fmt.Errorf("code index out of range")
		}

		// function index namespace starts with imported functions
		funcIdx := Index(importedFunctionCount + codeIndex)

		// Seek to see if there's a better name than "unknown"
		name := "unknown"
		for ; n < nLen; n++ {
			next := functionNames[n]
			if next.Index > funcIdx {
				break // we have function names, but starting at a later index
			} else if next.Index == funcIdx {
				name = next.Name
				break
			}
		}

		typeInstace, err := s.getTypeInstance(module.TypeSection[typeIndex])
		if err != nil {
			return rollbackFuncs, err
		}

		f := &FunctionInstance{
			Name:           name,
			FunctionType:   typeInstace,
			Body:           module.CodeSection[codeIndex].Body,
			LocalTypes:     module.CodeSection[codeIndex].LocalTypes,
			ModuleInstance: target,
		}

		if _, ok := analysisCache[codeIndex]; !ok {
			err := validateFunction(
				module, f, functionDeclarations, globalDeclarations,
				memoryDeclarations, tableDeclarations,
			)
			if err != nil {
				return rollbackFuncs, fmt.Errorf("invalid function at index %d/%d: %v", codeIndex, len(module.FunctionSection)-1, err)
			}
		}

		target.Functions = append(target.Functions, f)
		err = s.addFunctionInstance(f)
		if err != nil {
			return rollbackFuncs, err
		}
	}
	return rollbackFuncs, nil
}

func (s *Store) buildMemoryInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	// Allocate memory instances.
	for _, memSec := range module.MemorySection {
		if target.Memory != nil {
			// This case the memory instance is already imported,
			// and the current Wasm spec doesn't allow multiple memories.
			return rollbackFuncs, fmt.Errorf("multiple memories not supported")
		}
		target.Memory = &MemoryInstance{
			Buffer: make([]byte, memoryPagesToBytesNum(memSec.Min)),
			Min:    memSec.Min,
			Max:    memSec.Max,
		}
		s.Memories = append(s.Memories, target.Memory)
	}

	// Initialize the memory instance according to the Data section.
	for _, d := range module.DataSection {
		if target.Memory == nil {
			return rollbackFuncs, fmt.Errorf("unknown memory")
		} else if d.MemoryIndex != 0 {
			return rollbackFuncs, fmt.Errorf("memory index must be zero")
		}

		rawOffset, offsetType, err := s.executeConstExpression(target, d.OffsetExpression)
		if err != nil {
			return rollbackFuncs, fmt.Errorf("calculate offset: %w", err)
		} else if offsetType != ValueTypeI32 {
			return rollbackFuncs, fmt.Errorf("offset is not int32 but %T", offsetType)
		}

		offset, ok := rawOffset.(int32)
		if !ok {
			return rollbackFuncs, fmt.Errorf("offset is not int32 but 0x%x", offsetType)
		} else if offset < 0 {
			return rollbackFuncs, fmt.Errorf("offset must be positive int32: %d", offset)
		}

		size := uint64(offset) + uint64(len(d.Init))
		maxPage := uint32(memoryMaxPages)
		if int(d.MemoryIndex) < len(module.MemorySection) && module.MemorySection[d.MemoryIndex].Max != nil {
			maxPage = *module.MemorySection[d.MemoryIndex].Max
		}
		if size > memoryPagesToBytesNum(maxPage) {
			return rollbackFuncs, fmt.Errorf("memory size out of limit %d * 64Ki", int(*(module.MemorySection[d.MemoryIndex].Max)))
		}

		memoryInst := target.Memory
		if size > uint64(len(memoryInst.Buffer)) {
			return rollbackFuncs, fmt.Errorf("out of bounds memory access")
		}
		// Setup the rollback function before mutating the acutal memory.
		original := make([]byte, len(d.Init))
		copy(original, memoryInst.Buffer[offset:])
		rollbackFuncs = append(rollbackFuncs, func() {
			copy(memoryInst.Buffer[offset:], original)
		})
		copy(memoryInst.Buffer[offset:], d.Init)
	}
	return rollbackFuncs, nil
}

func (s *Store) buildTableInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	// Allocate table instances.
	for _, tableSeg := range module.TableSection {
		instance := newTableInstance(tableSeg.Limit.Min, tableSeg.Limit.Max)
		target.Tables = append(target.Tables, instance)
		s.Tables = append(s.Tables, instance)
	}

	for _, elem := range module.ElementSection {
		if elem.TableIndex >= Index(len(target.Tables)) {
			return rollbackFuncs, fmt.Errorf("index out of range of index space")
		}

		rawOffset, offsetType, err := s.executeConstExpression(target, elem.OffsetExpr)
		if err != nil {
			return rollbackFuncs, fmt.Errorf("calculate offset: %w", err)
		} else if offsetType != ValueTypeI32 {
			return rollbackFuncs, fmt.Errorf("offset is not int32 but %T", offsetType)
		}

		offset32, ok := rawOffset.(int32)
		if !ok {
			return rollbackFuncs, fmt.Errorf("offset is not int32 but %T", offsetType)
		} else if offset32 < 0 {
			return rollbackFuncs, fmt.Errorf("offset must be positive int32 but %d", offset32)
		}

		offset := int(offset32)
		size := offset + len(elem.Init)

		max := uint32(math.MaxUint32)
		if int(elem.TableIndex) < len(module.TableSection) && module.TableSection[elem.TableIndex].Limit.Max != nil {
			max = *module.TableSection[elem.TableIndex].Limit.Max
		}

		if size > int(max) {
			return rollbackFuncs, fmt.Errorf("table size out of limit of %d", max)
		}

		tableInst := target.Tables[elem.TableIndex]
		if size > len(tableInst.Table) {
			return rollbackFuncs, fmt.Errorf("out of bounds table access %d > %v", size, tableInst.Min)
		}
		for i := range elem.Init {
			i := i
			elm := elem.Init[i]
			if elm >= uint32(len(target.Functions)) {
				return rollbackFuncs, fmt.Errorf("unknown function specified by element")
			}
			// Setup the rollback function before mutating the table instance.
			pos := i + offset
			original := tableInst.Table[pos]
			rollbackFuncs = append(rollbackFuncs, func() {
				tableInst.Table[pos] = original
			})
			targetFunc := target.Functions[elm]
			tableInst.Table[pos] = TableElement{
				FunctionAddress: targetFunc.Address,
				FunctionTypeID:  targetFunc.FunctionType.TypeID,
			}
		}
	}
	if len(target.Tables) > 1 {
		return rollbackFuncs, fmt.Errorf("multiple tables not supported")
	}
	return rollbackFuncs, nil
}

func (s *Store) buildExportInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	target.Exports = make(map[string]*ExportInstance, len(module.ExportSection))
	for name, exp := range module.ExportSection {
		index := exp.Index
		var ei *ExportInstance
		switch exp.Kind {
		case ExportKindFunc:
			if index >= uint32(len(target.Functions)) {
				return nil, fmt.Errorf("unknown function for export[%s]", name)
			}
			ei = &ExportInstance{Kind: exp.Kind, Function: target.Functions[index]}
		case ExportKindGlobal:
			if index >= uint32(len(target.Globals)) {
				return nil, fmt.Errorf("unknown global for export[%s]", name)
			}
			ei = &ExportInstance{Kind: exp.Kind, Global: target.Globals[index]}
		case ExportKindMemory:
			if index != 0 || target.Memory == nil {
				return nil, fmt.Errorf("unknown memory for export[%s]", name)
			}
			ei = &ExportInstance{Kind: exp.Kind, Memory: target.Memory}
		case ExportKindTable:
			if index >= uint32(len(target.Tables)) {
				return nil, fmt.Errorf("unknown table for export[%s]", name)
			}
			ei = &ExportInstance{Kind: exp.Kind, Table: target.Tables[index]}
		}
		if err = target.addExport(exp.Name, ei); err != nil {
			return nil, err
		}
	}
	return
}

type valueTypeStack struct {
	stack       []ValueType
	stackLimits []int
}

const (
	// Only used in the anlyzeFunction below.
	valueTypeUnknown = ValueType(0xFF)
)

func (s *valueTypeStack) pop() (ValueType, error) {
	limit := 0
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	if len(s.stack) <= limit {
		return 0, fmt.Errorf("invalid operation: trying to pop at %d with limit %d",
			len(s.stack), limit)
	} else if len(s.stack) == limit+1 && s.stack[limit] == valueTypeUnknown {
		return valueTypeUnknown, nil
	} else {
		ret := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		return ret, nil
	}
}

func (s *valueTypeStack) popAndVerifyType(expected ValueType) error {
	actual, err := s.pop()
	if err != nil {
		return err
	}
	if actual != expected && actual != valueTypeUnknown && expected != valueTypeUnknown {
		return fmt.Errorf("type mismatch")
	}
	return nil
}

func (s *valueTypeStack) push(v ValueType) {
	s.stack = append(s.stack, v)
}

func (s *valueTypeStack) unreachable() {
	s.resetAtStackLimit()
	s.stack = append(s.stack, valueTypeUnknown)
}

func (s *valueTypeStack) resetAtStackLimit() {
	if len(s.stackLimits) != 0 {
		s.stack = s.stack[:s.stackLimits[len(s.stackLimits)-1]]
	} else {
		s.stack = []ValueType{}
	}
}

func (s *valueTypeStack) popStackLimit() {
	if len(s.stackLimits) != 0 {
		s.stackLimits = s.stackLimits[:len(s.stackLimits)-1]
	}
}

func (s *valueTypeStack) pushStackLimit() {
	s.stackLimits = append(s.stackLimits, len(s.stack))
}

func (s *valueTypeStack) popResults(expResults []ValueType, checkAboveLimit bool) error {
	limit := 0
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	for _, exp := range expResults {
		if err := s.popAndVerifyType(exp); err != nil {
			return err
		}
	}
	if checkAboveLimit {
		if !(limit == len(s.stack) || (limit+1 == len(s.stack) && s.stack[limit] == valueTypeUnknown)) {
			return fmt.Errorf("leftovers found in the stack")
		}
	}
	return nil
}

func (s *valueTypeStack) String() string {
	var typeStrs, limits []string
	for _, v := range s.stack {
		var str string
		if v == valueTypeUnknown {
			str = "unknown"
		} else if v == ValueTypeI32 {
			str = "i32"
		} else if v == ValueTypeI64 {
			str = "i64"
		} else if v == ValueTypeF32 {
			str = "f32"
		} else if v == ValueTypeF64 {
			str = "f64"
		}
		typeStrs = append(typeStrs, str)
	}
	for _, d := range s.stackLimits {
		limits = append(limits, fmt.Sprintf("%d", d))
	}
	return fmt.Sprintf("{stack: [%s], limits: [%s]}",
		strings.Join(typeStrs, ", "), strings.Join(limits, ","))
}

type functionBlock struct {
	StartAt, ElseAt, EndAt uint64
	BlockType              *FunctionType
	BlockTypeBytes         uint64
	IsLoop                 bool
	IsIf                   bool
}

// validateFunction validates the instruction sequence of a function instance body
// following the specification https://www.w3.org/TR/wasm-core-1/#instructions%E2%91%A2.
//
// TODO: put this in a separate file like validate.go.
func validateFunction(
	module *Module,
	f *FunctionInstance,
	functionDeclarations []Index,
	globalDeclarations []*GlobalType,
	memoryDeclarations []*MemoryType,
	tableDeclarations []*TableType,
) error {
	labelStack := []*functionBlock{
		{BlockType: f.FunctionType.Type, StartAt: math.MaxUint64},
	}
	valueTypeStack := &valueTypeStack{}
	for pc := uint64(0); pc < uint64(len(f.Body)); pc++ {
		op := f.Body[pc]
		if OpcodeI32Load <= op && op <= OpcodeI64Store32 {
			if len(memoryDeclarations) == 0 {
				return fmt.Errorf("unknown memory access")
			}
			pc++
			align, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read memory align: %v", err)
			}
			switch op {
			case OpcodeI32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeI32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeF32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeF64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI32Load8S:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Load8U:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Load8S, OpcodeI64Load8U:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI32Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI32Load16S, OpcodeI32Load16U:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Load16S, OpcodeI64Load16U:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI32Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Load32S, OpcodeI64Load32U:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64Store32:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			}
			pc += num
			// offset
			_, num, err = leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read memory offset: %v", err)
			}
			pc += num - 1
		} else if OpcodeMemorySize <= op && op <= OpcodeMemoryGrow {
			if len(memoryDeclarations) == 0 {
				return fmt.Errorf("unknown memory access")
			}
			pc++
			val, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			if val != 0 || num != 1 {
				return fmt.Errorf("memory instruction reserved bytes not zero with 1 byte")
			}
			switch Opcode(op) {
			case OpcodeMemoryGrow:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeMemorySize:
				valueTypeStack.push(ValueTypeI32)
			}
			pc += num - 1
		} else if OpcodeI32Const <= op && op <= OpcodeF64Const {
			pc++
			switch Opcode(op) {
			case OpcodeI32Const:
				_, num, err := leb128.DecodeInt32(bytes.NewBuffer(f.Body[pc:]))
				if err != nil {
					return fmt.Errorf("read i32 immediate: %s", err)
				}
				pc += num - 1
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Const:
				_, num, err := leb128.DecodeInt64(bytes.NewBuffer(f.Body[pc:]))
				if err != nil {
					return fmt.Errorf("read i64 immediate: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
				pc += num - 1
			case OpcodeF32Const:
				valueTypeStack.push(ValueTypeF32)
				pc += 3
			case OpcodeF64Const:
				valueTypeStack.push(ValueTypeF64)
				pc += 7
			}
		} else if OpcodeLocalGet <= op && op <= OpcodeGlobalSet {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			switch op {
			case OpcodeLocalGet:
				inputLen := uint32(len(f.FunctionType.Type.Params))
				if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.get %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				if index < inputLen {
					valueTypeStack.push(f.FunctionType.Type.Params[index])
				} else {
					valueTypeStack.push(f.LocalTypes[index-inputLen])
				}
			case OpcodeLocalSet:
				inputLen := uint32(len(f.FunctionType.Type.Params))
				if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.set %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.FunctionType.Type.Params[index]
				} else {
					expType = f.LocalTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
			case OpcodeLocalTee:
				inputLen := uint32(len(f.FunctionType.Type.Params))
				if l := uint32(len(f.LocalTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.tee %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.FunctionType.Type.Params[index]
				} else {
					expType = f.LocalTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
				valueTypeStack.push(expType)
			case OpcodeGlobalGet:
				if index >= uint32(len(globalDeclarations)) {
					return fmt.Errorf("invalid global index")
				}
				valueTypeStack.push(globalDeclarations[index].ValType)
			case OpcodeGlobalSet:
				if index >= uint32(len(globalDeclarations)) {
					return fmt.Errorf("invalid global index")
				} else if !globalDeclarations[index].Mutable {
					return fmt.Errorf("globa.set on immutable global type")
				} else if err := valueTypeStack.popAndVerifyType(
					globalDeclarations[index].ValType); err != nil {
					return err
				}
			}
		} else if op == OpcodeBr {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			} else if int(index) >= len(labelStack) {
				return fmt.Errorf("invalid br operation: index out of range")
			}
			pc += num - 1
			// Check type soundness.
			target := labelStack[len(labelStack)-int(index)-1]
			targetResultType := target.BlockType.Results
			if target.IsLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err := valueTypeStack.popResults(targetResultType, false); err != nil {
				return fmt.Errorf("type mismatch on the br operation: %v", err)
			}
			// br instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeBrIf {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			} else if int(index) >= len(labelStack) {
				return fmt.Errorf(
					"invalid ln param given for br_if: index=%d with %d for the current lable stack length",
					index, len(labelStack))
			}
			pc += num - 1
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for br_if")
			}
			// Check type soundness.
			target := labelStack[len(labelStack)-int(index)-1]
			targetResultType := target.BlockType.Results
			if target.IsLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err := valueTypeStack.popResults(targetResultType, false); err != nil {
				return fmt.Errorf("type mismatch on the br_if operation: %v", err)
			}
			// Push back the result
			for _, t := range targetResultType {
				valueTypeStack.push(t)
			}
		} else if op == OpcodeBrTable {
			pc++
			r := bytes.NewBuffer(f.Body[pc:])
			nl, num, err := leb128.DecodeUint32(r)
			if err != nil {
				return fmt.Errorf("read immediate: %w", err)
			}

			list := make([]uint32, nl)
			for i := uint32(0); i < nl; i++ {
				l, n, err := leb128.DecodeUint32(r)
				if err != nil {
					return fmt.Errorf("read immediate: %w", err)
				}
				num += n
				list[i] = l
			}
			ln, n, err := leb128.DecodeUint32(r)
			if err != nil {
				return fmt.Errorf("read immediate: %w", err)
			} else if int(ln) >= len(labelStack) {
				return fmt.Errorf(
					"invalid ln param given for br_table: ln=%d with %d for the current lable stack length",
					ln, len(labelStack))
			}
			pc += n + num - 1
			// Check type soundness.
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for br_table")
			}
			lnLabel := labelStack[len(labelStack)-1-int(ln)]
			expType := lnLabel.BlockType.Results
			if lnLabel.IsLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				expType = []ValueType{}
			}
			for _, l := range list {
				if int(l) >= len(labelStack) {
					return fmt.Errorf("invalid l param given for br_table")
				}
				label := labelStack[len(labelStack)-1-int(l)]
				expType2 := label.BlockType.Results
				if label.IsLoop {
					// Loop operation doesn't require results since the continuation is
					// the beginning of the loop.
					expType2 = []ValueType{}
				}
				if len(expType) != len(expType2) {
					return fmt.Errorf("incosistent block type length for br_table at %d; %v (ln=%d) != %v (l=%d)", l, expType, ln, expType2, l)
				}
				for i := range expType {
					if expType[i] != expType2[i] {
						return fmt.Errorf("incosistent block type for br_table at %d", l)
					}
				}
			}
			if err := valueTypeStack.popResults(expType, false); err != nil {
				return fmt.Errorf("type mismatch on the br_table operation: %v", err)
			}
			// br_table instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeCall {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			if int(index) >= len(functionDeclarations) {
				return fmt.Errorf("invalid function index")
			}
			funcType := module.TypeSection[functionDeclarations[index]]
			for i := 0; i < len(funcType.Params); i++ {
				if err := valueTypeStack.popAndVerifyType(funcType.Params[len(funcType.Params)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call operation param type")
				}
			}
			for _, exp := range funcType.Results {
				valueTypeStack.push(exp)
			}
		} else if op == OpcodeCallIndirect {
			pc++
			typeIndex, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			pc++
			if f.Body[pc] != 0x00 {
				return fmt.Errorf("call_indirect reserved bytes not zero but got %d", f.Body[pc])
			}
			if len(tableDeclarations) == 0 {
				return fmt.Errorf("table not given while having call_indirect")
			}
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the in table index's type for call_indirect")
			}
			if int(typeIndex) >= len(module.TypeSection) {
				return fmt.Errorf("invalid type index at call_indirect: %d", typeIndex)
			}
			funcType := module.TypeSection[typeIndex]
			for i := 0; i < len(funcType.Params); i++ {
				if err := valueTypeStack.popAndVerifyType(funcType.Params[len(funcType.Params)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call_indirect operation input type")
				}
			}
			for _, exp := range funcType.Results {
				valueTypeStack.push(exp)
			}
		} else if OpcodeI32Eqz <= op && op <= OpcodeF64ReinterpretI64 {
			switch Opcode(op) {
			case OpcodeI32Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Eq, OpcodeI32Ne, OpcodeI32LtS,
				OpcodeI32LtU, OpcodeI32GtS, OpcodeI32GtU, OpcodeI32LeS,
				OpcodeI32LeU, OpcodeI32GeS, OpcodeI32GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eq, OpcodeI64Ne, OpcodeI64LtS,
				OpcodeI64LtU, OpcodeI64GtS, OpcodeI64GtU,
				OpcodeI64LeS, OpcodeI64LeU, OpcodeI64GeS, OpcodeI64GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF32Eq, OpcodeF32Ne, OpcodeF32Lt, OpcodeF32Gt, OpcodeF32Le, OpcodeF32Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF64Eq, OpcodeF64Ne, OpcodeF64Lt, OpcodeF64Gt, OpcodeF64Le, OpcodeF64Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Clz, OpcodeI32Ctz, OpcodeI32Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Add, OpcodeI32Sub, OpcodeI32Mul, OpcodeI32DivS,
				OpcodeI32DivU, OpcodeI32RemS, OpcodeI32RemU, OpcodeI32And,
				OpcodeI32Or, OpcodeI32Xor, OpcodeI32Shl, OpcodeI32ShrS,
				OpcodeI32ShrU, OpcodeI32Rotl, OpcodeI32Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Clz, OpcodeI64Ctz, OpcodeI64Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64Add, OpcodeI64Sub, OpcodeI64Mul, OpcodeI64DivS,
				OpcodeI64DivU, OpcodeI64RemS, OpcodeI64RemU, OpcodeI64And,
				OpcodeI64Or, OpcodeI64Xor, OpcodeI64Shl, OpcodeI64ShrS,
				OpcodeI64ShrU, OpcodeI64Rotl, OpcodeI64Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32Abs, OpcodeF32Neg, OpcodeF32Ceil,
				OpcodeF32Floor, OpcodeF32Trunc, OpcodeF32Nearest,
				OpcodeF32Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32Add, OpcodeF32Sub, OpcodeF32Mul,
				OpcodeF32Div, OpcodeF32Min, OpcodeF32Max,
				OpcodeF32Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64Abs, OpcodeF64Neg, OpcodeF64Ceil,
				OpcodeF64Floor, OpcodeF64Trunc, OpcodeF64Nearest,
				OpcodeF64Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64Add, OpcodeF64Sub, OpcodeF64Mul,
				OpcodeF64Div, OpcodeF64Min, OpcodeF64Max,
				OpcodeF64Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32WrapI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.wrap_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF32S, OpcodeI32TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF64S, OpcodeI32TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ExtendI32S, OpcodeI64ExtendI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF32S, OpcodeI64TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF64S, OpcodeI64TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ConvertI32s, OpcodeF32ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32ConvertI64S, OpcodeF32ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32DemoteF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.demote_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ConvertI32S, OpcodeF64ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64ConvertI64S, OpcodeF64ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64PromoteF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.promote_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32ReinterpretF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.reinterpret_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ReinterpretF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.reinterpret_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ReinterpretI32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.reinterpret_i32: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ReinterpretI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.reinterpret_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			default:
				return fmt.Errorf("invalid numeric instruction 0x%x", op)
			}
		} else if op == OpcodeBlock {
			bt, num, err := DecodeBlockType(f.ModuleInstance.Types, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &functionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeLoop {
			bt, num, err := DecodeBlockType(f.ModuleInstance.Types, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &functionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
				IsLoop:         true,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeIf {
			bt, num, err := DecodeBlockType(f.ModuleInstance.Types, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &functionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
				IsIf:           true,
			})
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the operand for 'if': %v", err)
			}
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeElse {
			bl := labelStack[len(labelStack)-1]
			bl.ElseAt = pc
			// Check the type soundness of the instructions *before*　 entering this Eles Op.
			if err := valueTypeStack.popResults(bl.BlockType.Results, true); err != nil {
				return fmt.Errorf("invalid instruction results in then instructions")
			}
			// Before entring instructions inside else, we pop all the values pushed by
			// then block.
			valueTypeStack.resetAtStackLimit()
		} else if op == OpcodeEnd {
			bl := labelStack[len(labelStack)-1]
			bl.EndAt = pc
			labelStack = labelStack[:len(labelStack)-1]
			if bl.IsIf && bl.ElseAt <= bl.StartAt {
				if len(bl.BlockType.Results) > 0 {
					return fmt.Errorf("type mismatch between then and else blocks")
				}
				// To handle if block without else properly,
				// we set ElseAt to EndAt-1 so we can just skip else.
				bl.ElseAt = bl.EndAt - 1
			}
			// Check type soundness.
			if err := valueTypeStack.popResults(bl.BlockType.Results, true); err != nil {
				return fmt.Errorf("invalid instruction results at end instruction; expected %v: %v", bl.BlockType.Results, err)
			}
			// Put the result types at the end after resetting at the stack limit
			// since we might have Any type between the limit and the current top.
			valueTypeStack.resetAtStackLimit()
			for _, exp := range bl.BlockType.Results {
				valueTypeStack.push(exp)
			}
			// We exit if/loop/block, so reset the constraints on the stack manipulation
			// on values previously pushed by outer blocks.
			valueTypeStack.popStackLimit()
		} else if op == OpcodeReturn {
			expTypes := f.FunctionType.Type.Results
			for i := 0; i < len(expTypes); i++ {
				if err := valueTypeStack.popAndVerifyType(expTypes[len(expTypes)-1-i]); err != nil {
					return fmt.Errorf("return type mismatch on return: %v; want %v", err, expTypes)
				}
			}
			// return instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeDrop {
			_, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid drop: %v", err)
			}
		} else if op == OpcodeSelect {
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("type mismatch on 3rd select operand: %v", err)
			}
			v1, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid select: %v", err)
			}
			v2, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid select: %v", err)
			}
			if v1 != v2 && v1 != valueTypeUnknown && v2 != valueTypeUnknown {
				return fmt.Errorf("type mismatch on 1st and 2nd select operands")
			}
			if v1 == valueTypeUnknown {
				valueTypeStack.push(v2)
			} else {
				valueTypeStack.push(v1)
			}
		} else if op == OpcodeUnreachable {
			// unreachable instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeNop {
		} else {
			return fmt.Errorf("invalid instruction 0x%x", op)
		}
	}

	if len(labelStack) > 0 {
		return fmt.Errorf("ill-nested block exists")
	}

	return nil
}

// DecodeBlockType is exported for use in the compiler
func DecodeBlockType(types []*TypeInstance, r io.Reader) (*FunctionType, uint64, error) {
	raw, num, err := leb128.DecodeInt33AsInt64(r)
	if err != nil {
		return nil, 0, fmt.Errorf("decode int33: %w", err)
	}

	var ret *FunctionType
	switch raw {
	case -64: // 0x40 in original byte = nil
		ret = &FunctionType{}
	case -1: // 0x7f in original byte = i32
		ret = &FunctionType{Results: []ValueType{ValueTypeI32}}
	case -2: // 0x7e in original byte = i64
		ret = &FunctionType{Results: []ValueType{ValueTypeI64}}
	case -3: // 0x7d in original byte = f32
		ret = &FunctionType{Results: []ValueType{ValueTypeF32}}
	case -4: // 0x7c in original byte = f64
		ret = &FunctionType{Results: []ValueType{ValueTypeF64}}
	default:
		if raw < 0 || (raw >= int64(len(types))) {
			return nil, 0, fmt.Errorf("invalid block type: %d", raw)
		}
		ret = types[raw].Type
	}
	return ret, num, nil
}

// HostFunctionCallContext is the first argument of all host functions.
type HostFunctionCallContext struct {
	// Memory is the currently used memory instance at the time when the host function call is made.
	Memory *MemoryInstance
	// TODO: Add others if necessary.
}

func (s *Store) AddHostFunction(moduleName, funcName string, fn reflect.Value) error {
	getTypeOf := func(kind reflect.Kind) (ValueType, error) {
		switch kind {
		case reflect.Float64:
			return ValueTypeF64, nil
		case reflect.Float32:
			return ValueTypeF32, nil
		case reflect.Int32, reflect.Uint32:
			return ValueTypeI32, nil
		case reflect.Int64, reflect.Uint64:
			return ValueTypeI64, nil
		default:
			return 0x00, fmt.Errorf("invalid type: %s", kind.String())
		}
	}
	getType := func(p reflect.Type) (*FunctionType, error) {
		var err error
		if p.NumIn() == 0 {
			return nil, fmt.Errorf("host function must accept *wasm.HostFunctionCallContext as the first param")
		}
		paramTypes := make([]ValueType, p.NumIn()-1)
		for i := range paramTypes {
			paramTypes[i], err = getTypeOf(p.In(i + 1).Kind())
			if err != nil {
				return nil, err
			}
		}

		resultTypes := make([]ValueType, p.NumOut())
		for i := range resultTypes {
			resultTypes[i], err = getTypeOf(p.Out(i).Kind())
			if err != nil {
				return nil, err
			}
		}
		return &FunctionType{Params: paramTypes, Results: resultTypes}, nil
	}

	m := s.getModuleInstance(moduleName)

	sig, err := getType(fn.Type())
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	typeInstace, err := s.getTypeInstance(sig)
	if err != nil {
		return err
	}

	f := &FunctionInstance{
		Name:           fmt.Sprintf("%s.%s", moduleName, funcName),
		HostFunction:   &fn,
		FunctionType:   typeInstace,
		ModuleInstance: m,
	}

	if err = s.engine.Compile(f); err != nil {
		return fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}
	if err = s.addFunctionInstance(f); err != nil {
		return err
	}
	if err = m.addExport(funcName, &ExportInstance{Kind: ExportKindFunc, Function: f}); err != nil {
		s.Functions = s.Functions[:len(s.Functions)-1] // revert the add on conflict
		return err
	}
	return nil
}

func (s *Store) AddGlobal(moduleName, name string, value uint64, valueType ValueType, mutable bool) error {
	g := &GlobalInstance{
		Val:  value,
		Type: &GlobalType{Mutable: mutable, ValType: valueType},
	}
	s.Globals = append(s.Globals, g)

	m := s.getModuleInstance(moduleName)
	return m.addExport(name, &ExportInstance{Kind: ExportKindGlobal, Global: g})
}

func (s *Store) AddTableInstance(moduleName, name string, min uint32, max *uint32) error {
	t := newTableInstance(min, max)
	s.Tables = append(s.Tables, t)

	m := s.getModuleInstance(moduleName)
	return m.addExport(name, &ExportInstance{Kind: ExportKindTable, Table: t})
}

func (s *Store) AddMemoryInstance(moduleName, name string, min uint32, max *uint32) error {
	memory := &MemoryInstance{
		Buffer: make([]byte, memoryPagesToBytesNum(min)),
		Min:    min,
		Max:    max,
	}
	s.Memories = append(s.Memories, memory)
	m := s.getModuleInstance(moduleName)
	return m.addExport(name, &ExportInstance{Kind: ExportKindMemory, Memory: memory})
}

func (s *Store) getTypeInstance(t *FunctionType) (*TypeInstance, error) {
	key := t.String()
	id, ok := s.TypeIDs[key]
	if !ok {
		l := len(s.TypeIDs)
		if l >= s.maximumFunctionTypes {
			return nil, fmt.Errorf("too many function types in a store")
		}
		id = FunctionTypeID(len(s.TypeIDs))
		s.TypeIDs[key] = id
	}
	return &TypeInstance{Type: t, TypeID: id}, nil
}

// getModuleInstance returns an existing ModuleInstance if exists, or assigns a new one.
func (s *Store) getModuleInstance(name string) *ModuleInstance {
	m, ok := s.ModuleInstances[name]
	if !ok {
		m = &ModuleInstance{Name: name, Exports: map[string]*ExportInstance{}}
		s.ModuleInstances[name] = m
	}
	return m
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

// UninitializedTableElementTypeID math.MaxUint64 to represent the uninitialized elements.
var UninitializedTableElementTypeID FunctionTypeID = math.MaxUint32
