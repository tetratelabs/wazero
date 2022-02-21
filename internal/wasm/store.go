package internalwasm

import (
	"bytes"
	"fmt"
	"io"
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
	// See https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0
	Store struct {
		// The following fields are wazero-specific fields of Store.

		// Engine is a global context for a Store which is in responsible for compilation and execution of Wasm modules.
		Engine Engine

		// ModuleInstances holds the instantiated Wasm modules by module name from Instantiate.
		ModuleInstances map[string]*ModuleInstance

		// hostExports holds host functions by module name from ExportHostFunctions.
		hostExports map[string]*HostExports

		// ModuleContexts holds default host function call contexts keyed by module name.
		ModuleContexts map[string]*ModuleContext

		// TypeIDs maps each FunctionType.String() to a unique FunctionTypeID. This is used at runtime to
		// do type-checks on indirect function calls.
		TypeIDs map[string]FunctionTypeID

		// maximumFunctionAddress represents the limit on the number of function addresses (= function instances) in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionAddress int
		//  maximumFunctionTypes represents the limit on the number of function types in a store.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumFunctionTypes int
		// maximumGlobals is the maximum number of globals that can be declared in a module.
		// Note: this is fixed to 2^27 but have this a field for testability.
		maximumGlobals int

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
		// FunctionKind describes how this function should be called.
		FunctionKind FunctionKind
		// HostFunction holds the runtime representation of host functions.
		// This is nil when FunctionKind == FunctionKindWasm. Otherwise, all the above fields are ignored as they are
		// specific to Wasm functions.
		HostFunction *reflect.Value
		// Address is the funcaddr(https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr) of this function instance.
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
	//
	// Note: In WebAssembly 1.0 (MVP), there may be up to one Memory per store, which means the precise memory is always
	// wasm.Store Memories index zero: `store.Memories[0]`
	// See https://www.w3.org/TR/wasm-core-1/#memory-instances%E2%91%A0.
	MemoryInstance struct {
		Buffer []byte
		Min    uint32
		Max    *uint32
	}

	// FunctionAddress is funcaddr (https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr),
	// and the index to Store.Functions.
	FunctionAddress uint64

	// FunctionTypeID is a uniquely assigned integer for a function type.
	// This is wazero specific runtime object and specific to a store,
	// and used at runtime to do type-checks on indirect function calls.
	FunctionTypeID uint32
)

// The wazero specific limitations described at RATIONALE.md.
const (
	maximumFunctionAddress = 1 << 27
	maximumFunctionTypes   = 1 << 27
	maximumGlobals         = 1 << 27
	maximumValuesOnStack   = 1 << 27
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
		return nil, fmt.Errorf("export %q in module %q is a %s, not a %s", name, m.Name, ExportKindName(exp.Kind), ExportKindName(kind))
	}
	return exp, nil
}

func NewStore(engine Engine) *Store {
	return &Store{
		ModuleInstances:        map[string]*ModuleInstance{},
		ModuleContexts:         map[string]*ModuleContext{},
		TypeIDs:                map[string]FunctionTypeID{},
		Engine:                 engine,
		maximumFunctionAddress: maximumFunctionAddress,
		maximumFunctionTypes:   maximumFunctionTypes,
		maximumGlobals:         maximumGlobals,
	}
}

func (s *Store) Instantiate(module *Module, name string) (*ModuleContext, error) {
	if err := s.requireModuleUnused(name); err != nil {
		return nil, err
	}

	// Check the start function is valid.
	// TODO: this should be verified during decode so that errors have the correct source positions
	if module.StartSection != nil {
		startIndex := *module.StartSection
		ft := module.TypeOfFunction(startIndex)
		if ft == nil { // TODO: move this check to that a module can never be decoded invalidly
			return nil, fmt.Errorf("module[%s] start function has an invalid type", name)
		}
		if len(ft.Params) > 0 || len(ft.Results) > 0 {
			return nil, fmt.Errorf("module[%s] start function must have an empty (nullary) signature: %s", name, ft.String())
		}
	}

	instance := &ModuleInstance{Name: name}
	for _, t := range module.TypeSection {
		typeInstance, err := s.getTypeInstance(t)
		if err != nil {
			return nil, err
		}
		instance.Types = append(instance.Types, typeInstance)
	}

	s.ModuleInstances[name] = instance
	// Resolve the imports before doing the actual instantiation (mutating store).
	if err := s.resolveImports(module, instance); err != nil {
		return nil, fmt.Errorf("resolve imports: %w", err)
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
		return nil, fmt.Errorf("globals: %w", err)
	}
	rs, err = s.buildFunctionInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return nil, fmt.Errorf("functions: %w", err)
	}
	rs, err = s.buildTableInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return nil, fmt.Errorf("tables: %w", err)
	}
	rs, err = s.buildMemoryInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return nil, fmt.Errorf("memories: %w", err)
	}
	rs, err = s.buildExportInstances(module, instance)
	rollbackFuncs = append(rollbackFuncs, rs...)
	if err != nil {
		return nil, fmt.Errorf("exports: %w", err)
	}

	for i, f := range instance.Functions {
		if err := s.Engine.Compile(f); err != nil {
			idx := module.SectionElementCount(SectionIDFunction) - 1
			return nil, fmt.Errorf("compilation failed at index %d/%d: %v", i, idx, err)
		}
	}

	// Build the default context for calls to this module
	ctx := NewModuleContext(s, instance)
	s.ModuleContexts[name] = ctx

	// Now we are safe to finalize the state.
	rollbackFuncs = nil

	// Execute the start function.
	if module.StartSection != nil {
		funcIdx := *module.StartSection
		if _, err = s.Engine.Call(ctx, instance.Functions[funcIdx]); err != nil {
			return nil, fmt.Errorf("module[%s] start function failed: %w", name, err)
		}
	}
	return ctx, nil
}

// ModuleExports implements wasm.Store ModuleExports
func (s *Store) ModuleExports(moduleName string) (publicwasm.ModuleExports, bool) {
	e, ok := s.ModuleContexts[moduleName]
	return e, ok
}

// HostExports implements wasm.Store HostExports
func (s *Store) HostExports(moduleName string) (publicwasm.HostExports, bool) {
	e, ok := s.hostExports[moduleName]
	return e, ok
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

func executeConstExpression(globals []*GlobalInstance, expr *ConstantExpression) (v interface{}, valueType ValueType, err error) {
	r := bytes.NewBuffer(expr.Data)
	switch expr.Opcode {
	case OpcodeI32Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read i32: %w", err)
		}
		return v, ValueTypeI32, nil
	case OpcodeI64Const:
		v, _, err = leb128.DecodeInt64(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read i64: %w", err)
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
		if uint32(len(globals)) <= id {
			return nil, 0, fmt.Errorf("global index out of range")
		}
		g := globals[id]
		switch g.Type.ValType {
		case ValueTypeI32:
			v = int32(g.Val)
			return v, ValueTypeI32, nil
		case ValueTypeI64:
			v = int64(g.Val)
			return v, ValueTypeI64, nil
		case ValueTypeF32:
			v = publicwasm.DecodeF32(g.Val)
			return v, ValueTypeF32, nil
		case ValueTypeF64:
			v = publicwasm.DecodeF64(g.Val)
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

	// We limit the number of globals in a moudle to 2^27.
	globalDecls := len(module.GlobalSection)
	for _, imp := range module.ImportSection {
		if imp.Kind == ImportKindGlobal {
			globalDecls++
		}
	}
	if globalDecls > s.maximumGlobals {
		return rollbackFuncs, fmt.Errorf("too many globals in a module")
	}

	for _, gs := range module.GlobalSection {
		raw, t, err := executeConstExpression(target.Globals, gs.Init)
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
			gv = publicwasm.EncodeF32(v)
		case float64:
			gv = publicwasm.EncodeF64(v)
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

	funcs, globals, mems, tables := module.allDeclarations()

	var functionNames NameMap
	if module.NameSection != nil {
		functionNames = module.NameSection.FunctionNames
	}

	n, nLen := 0, len(functionNames)

	for codeIndex, typeIndex := range module.FunctionSection {
		if typeIndex >= module.SectionElementCount(SectionIDType) {
			return rollbackFuncs, fmt.Errorf("function type index out of range")
		} else if uint32(codeIndex) >= module.SectionElementCount(SectionIDCode) {
			return rollbackFuncs, fmt.Errorf("code index out of range")
		}

		funcIdx := Index(len(target.Functions))
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

		typeInstance, err := s.getTypeInstance(module.TypeSection[typeIndex])
		if err != nil {
			return rollbackFuncs, err
		}

		f := &FunctionInstance{
			Name:           name,
			FunctionKind:   FunctionKindWasm,
			FunctionType:   typeInstance,
			Body:           module.CodeSection[codeIndex].Body,
			LocalTypes:     module.CodeSection[codeIndex].LocalTypes,
			ModuleInstance: target,
		}

		if err := validateFunctionInstance(f, funcs, globals, mems, tables, module.TypeSection, maximumValuesOnStack); err != nil {
			idx := module.SectionElementCount(SectionIDFunction) - 1
			return rollbackFuncs, fmt.Errorf("invalid function '%s' (%d/%d): %v", f.Name, codeIndex, idx, err)
		}

		err = s.addFunctionInstance(f)
		if err != nil {
			return rollbackFuncs, err
		}
		target.Functions = append(target.Functions, f)
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

		rawOffset, offsetType, err := executeConstExpression(target.Globals, d.OffsetExpression)
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
		maxPage := MemoryMaxPages
		if d.MemoryIndex < module.SectionElementCount(SectionIDMemory) && module.MemorySection[d.MemoryIndex].Max != nil {
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

		rawOffset, offsetType, err := executeConstExpression(target.Globals, elem.OffsetExpr)
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
		if elem.TableIndex < module.SectionElementCount(SectionIDTable) && module.TableSection[elem.TableIndex].Limit.Max != nil {
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
	target.Exports = make(map[string]*ExportInstance, module.SectionElementCount(SectionIDExport))
	for name, exp := range module.ExportSection {
		index := exp.Index
		var ei *ExportInstance
		switch exp.Kind {
		case ExportKindFunc:
			if index >= uint32(len(target.Functions)) {
				return nil, fmt.Errorf("unknown function for export[%s]", name)
			}
			ei = &ExportInstance{Kind: exp.Kind, Function: target.Functions[index]}
			// The module instance of the host function is a fake that only includes the function and its types.
			// We need to assign the ModuleInstance when re-exporting so that any memory defined in the target is
			// available to the wasm.ModuleContext Memory.
			if ei.Function.HostFunction != nil {
				ei.Function.ModuleInstance = target
			}
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

// AddHostFunction exports a function so that it can be imported under the given module and name. If a function already
// exists for this module and name it is ignored rather than overwritten.
//
// Note: The wasm.Memory of the fn will be from the importing module.
// Note: The ModuleInstance of this host function is lazy created and only includes exported functions and their types.
func (s *Store) AddHostFunction(moduleName string, hf *GoFunc) (*FunctionInstance, error) {
	m := s.getModuleInstance(moduleName)
	typeInstance, err := s.getTypeInstance(hf.functionType)
	if err != nil {
		return nil, err
	}

	f := &FunctionInstance{
		Name:           fmt.Sprintf("%s.%s", moduleName, hf.wasmFunctionName),
		HostFunction:   hf.goFunc,
		FunctionKind:   hf.functionKind,
		FunctionType:   typeInstance,
		ModuleInstance: m,
	}

	if err = s.Engine.Compile(f); err != nil {
		return nil, fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}
	if err = s.addFunctionInstance(f); err != nil {
		return nil, err
	}
	// TODO: This races on adding export. A future design may be able to eliminate this race, possibly via a HostModule
	// type.
	if err = m.addExport(hf.wasmFunctionName, &ExportInstance{Kind: ExportKindFunc, Function: f}); err != nil {
		s.Functions = s.Functions[:len(s.Functions)-1] // lost race: revert the add on conflict
		return nil, err
	}
	return f, nil
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
