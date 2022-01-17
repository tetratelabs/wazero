package wasm

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

type (
	Store struct {
		engine          Engine
		ModuleInstances map[string]*ModuleInstance

		Functions []*FunctionInstance
		Globals   []*GlobalInstance
		Memories  []*MemoryInstance
		Tables    []*TableInstance
	}

	ModuleInstance struct {
		Exports   map[string]*ExportInstance
		Functions []*FunctionInstance
		Globals   []*GlobalInstance
		Memory    *MemoryInstance
		Tables    []*TableInstance

		Types []*FunctionType
	}

	ExportInstance struct {
		Kind     byte
		Function *FunctionInstance
		Global   *GlobalInstance
		Memory   *MemoryInstance
		Table    *TableInstance
	}

	FunctionInstance struct {
		Name           string
		ModuleInstance *ModuleInstance
		Body           []byte
		Signature      *FunctionType
		NumLocals      uint32
		LocalTypes     []ValueType
		HostFunction   *reflect.Value
	}

	HostFunctionCallContext struct {
		Memory *MemoryInstance
		// TODO: Add others if necessary.
	}

	FunctionInstanceBlock struct {
		StartAt, ElseAt, EndAt uint64
		BlockType              *FunctionType
		BlockTypeBytes         uint64
		IsLoop                 bool // TODO: might not be necessary
		IsIf                   bool // TODO: might not be necessary
	}

	GlobalInstance struct {
		Type *GlobalType
		Val  uint64
	}

	TableInstance struct {
		Table    []*TableInstanceElm
		Min      uint32
		Max      *uint32
		ElemType byte
	}

	TableInstanceElm struct {
		Function *FunctionInstance
		// Other ref types will be added.
	}

	MemoryInstance struct {
		Buffer []byte
		Min    uint32
		Max    *uint32
	}
)

func NewStore(engine Engine) *Store {
	return &Store{ModuleInstances: map[string]*ModuleInstance{}, engine: engine}
}

func (s *Store) Instantiate(module *Module, name string) error {
	instance := &ModuleInstance{Types: module.TypeSection}
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

	// We compile functions after successfully finished building all instances.
	// This is not only because we want to do early feedback on malicious binaries,
	// but also during the compilation phase, the compilers have to see all the possible
	// instances (function, memory, table) in the module instance.
	if err = s.engine.PreCompile(instance.Functions); err != nil {
		return fmt.Errorf("failed to precompile: %w", err)
	}
	for i, f := range instance.Functions {
		if err := s.engine.Compile(f); err != nil {
			return fmt.Errorf("compilation failed at index %d/%d: %v", i, len(module.FunctionSection)-1, err)
		}
	}

	// Check the start function is valid.
	if module.StartSection != nil {
		index := *module.StartSection
		if int(index) >= len(instance.Functions) {
			return fmt.Errorf("invalid start function index: %d", index)
		}
		signature := instance.Functions[index].Signature
		if len(signature.Params) != 0 || len(signature.Results) != 0 {
			return fmt.Errorf("start function must have the empty signature")
		}
	}

	// Now we are safe to finalize the state.
	rollbackFuncs = nil

	// Execute the start function.
	if module.StartSection != nil {
		f := instance.Functions[*module.StartSection]
		if _, err := s.engine.Call(f); err != nil {
			return fmt.Errorf("calling start function failed: %v", err)
		}
	}
	return nil
}

func (s *Store) CallFunction(moduleName, funcName string, params ...uint64) (results []uint64, resultTypes []ValueType, err error) {
	m, ok := s.ModuleInstances[moduleName]
	if !ok {
		return nil, nil, fmt.Errorf("module '%s' not instantiated", moduleName)
	}

	exp, ok := m.Exports[funcName]
	if !ok {
		return nil, nil, fmt.Errorf("exported function '%s' not found in '%s'", funcName, moduleName)
	}

	if exp.Kind != ExportKindFunction {
		return nil, nil, fmt.Errorf("'%s' is not functype", funcName)
	}

	f := exp.Function
	if len(f.Signature.Params) != len(params) {
		return nil, nil, fmt.Errorf("invalid number of parameters")
	}

	ret, err := s.engine.Call(f, params...)
	return ret, f.Signature.Results, err
}

func (s *Store) resolveImports(module *Module, target *ModuleInstance) error {
	for _, is := range module.ImportSection {
		if err := s.resolveImport(target, is); err != nil {
			return fmt.Errorf("%s: %w", is.Name, err)
		}
	}
	return nil
}

func (s *Store) resolveImport(target *ModuleInstance, is *ImportSegment) error {
	em, ok := s.ModuleInstances[is.Module]
	if !ok {
		return fmt.Errorf("failed to resolve import of module name %s", is.Module)
	}

	e, ok := em.Exports[is.Name]
	if !ok {
		return fmt.Errorf("not exported in module %s", is.Module)
	}

	if is.Desc.Kind != e.Kind {
		return fmt.Errorf("type mismatch on export: got %#x but want %#x", e.Kind, is.Desc.Kind)
	}
	switch is.Desc.Kind {
	case ImportKindFunction:
		if err := s.applyFunctionImport(target, is.Desc.FuncTypeIndex, e); err != nil {
			return fmt.Errorf("applyFunctionImport: %w", err)
		}
	case ImportKindTable:
		if err := s.applyTableImport(target, is.Desc.TableTypePtr, e); err != nil {
			return fmt.Errorf("applyTableImport: %w", err)
		}
	case ImportKindMemory:
		if err := s.applyMemoryImport(target, is.Desc.MemTypePtr, e); err != nil {
			return fmt.Errorf("applyMemoryImport: %w", err)
		}
	case ImportKindGlobal:
		if err := s.applyGlobalImport(target, is.Desc.GlobalTypePtr, e); err != nil {
			return fmt.Errorf("applyGlobalImport: %w", err)
		}
	default:
		return fmt.Errorf("invalid kind of import: %#x", is.Desc.Kind)
	}

	return nil
}

func (s *Store) applyFunctionImport(target *ModuleInstance, typeIndex uint32, externModuleExportIsntance *ExportInstance) error {
	f := externModuleExportIsntance.Function
	if int(typeIndex) >= len(target.Types) {
		return fmt.Errorf("unknown type for function import")
	}
	iSig := target.Types[typeIndex]
	if !HasSameSignature(iSig.Results, f.Signature.Results) {
		return fmt.Errorf("return signature mimatch: %#x != %#x", iSig.Results, f.Signature.Results)
	} else if !HasSameSignature(iSig.Params, f.Signature.Params) {
		return fmt.Errorf("input signature mimatch: %#x != %#x", iSig.Params, f.Signature.Params)
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
		v, err = readFloat32(r)
		if err != nil {
			return nil, 0, fmt.Errorf("read f32: %w", err)
		}
		return v, ValueTypeF32, nil
	case OpcodeF64Const:
		v, err = readFloat64(r)
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
			v = math.Float64frombits(uint64(g.Val))
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
	var functionDeclarations []uint32
	var globalDeclarations []*GlobalType
	var memoryDeclarations []*MemoryType
	var tableDeclarations []*TableType
	for _, imp := range module.ImportSection {
		switch imp.Desc.Kind {
		case ImportKindFunction:
			functionDeclarations = append(functionDeclarations, imp.Desc.FuncTypeIndex)
		case ImportKindGlobal:
			globalDeclarations = append(globalDeclarations, imp.Desc.GlobalTypePtr)
		case ImportKindMemory:
			memoryDeclarations = append(memoryDeclarations, imp.Desc.MemTypePtr)
		case ImportKindTable:
			tableDeclarations = append(tableDeclarations, imp.Desc.TableTypePtr)
		}
	}
	importedFunctionCount := len(functionDeclarations)
	functionDeclarations = append(functionDeclarations, module.FunctionSection...)
	for _, g := range module.GlobalSection {
		globalDeclarations = append(globalDeclarations, g.Type)
	}
	memoryDeclarations = append(memoryDeclarations, module.MemorySection...)
	tableDeclarations = append(tableDeclarations, module.TableSection...)

	var functionNames map[uint32]string
	if module.NameSection != nil && module.NameSection.FunctionNames != nil {
		functionNames = module.NameSection.FunctionNames
	} else {
		functionNames = map[uint32]string{}
	}

	analysisCache := map[int]map[uint64]struct{}{}
	for codeIndex, typeIndex := range module.FunctionSection {
		if typeIndex >= uint32(len(module.TypeSection)) {
			return rollbackFuncs, fmt.Errorf("function type index out of range")
		} else if codeIndex >= len(module.CodeSection) {
			return rollbackFuncs, fmt.Errorf("code index out of range")
		}

		name := getFunctionName(functionNames, importedFunctionCount, codeIndex)

		f := &FunctionInstance{
			Name:           name,
			Signature:      module.TypeSection[typeIndex],
			Body:           module.CodeSection[codeIndex].Body,
			NumLocals:      module.CodeSection[codeIndex].NumLocals,
			LocalTypes:     module.CodeSection[codeIndex].LocalTypes,
			ModuleInstance: target,
		}

		if _, ok := analysisCache[codeIndex]; !ok {
			err := analyzeFunction(
				module, f, functionDeclarations, globalDeclarations,
				memoryDeclarations, tableDeclarations,
			)
			if err != nil {
				return rollbackFuncs, fmt.Errorf("invalid function at index %d/%d: %v", codeIndex, len(module.FunctionSection)-1, err)
			}
		}

		target.Functions = append(target.Functions, f)
		s.Functions = append(s.Functions, f)
	}
	return rollbackFuncs, nil
}

// getFunctionName gets the name of the function corresponding to the codeIndex.
func getFunctionName(functionNames map[uint32]string, importedFunctionCount int, codeIndex int) string {
	// Per spec: "... function indices, starting with the smallest index not referencing a function import."
	// This means we have to add an offset of the imported function count to resolve the correct function index.
	// See https://www.w3.org/TR/wasm-core-1/#functions%E2%91%A0
	name, ok := functionNames[uint32(importedFunctionCount)+uint32(codeIndex)]
	if !ok {
		name = "unknown"
	}
	return name
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
			Buffer: make([]byte, uint64(memSec.Min)*PageSize),
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
		max := uint64(math.MaxUint32)
		if int(d.MemoryIndex) < len(module.MemorySection) && module.MemorySection[d.MemoryIndex].Max != nil {
			max = uint64(*module.MemorySection[d.MemoryIndex].Max)
		}
		if size > max*PageSize {
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
		tableInst := &TableInstance{
			Table:    make([]*TableInstanceElm, tableSeg.Limit.Min),
			Min:      tableSeg.Limit.Min,
			Max:      tableSeg.Limit.Max,
			ElemType: tableSeg.ElemType,
		}
		target.Tables = append(target.Tables, tableInst)
		s.Tables = append(s.Tables, tableInst)
	}

	for _, elem := range module.ElementSection {
		if elem.TableIndex >= uint32(len(target.Tables)) {
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
			tableInst.Table[pos] = &TableInstanceElm{
				Function: target.Functions[elm],
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
		index := int(exp.Desc.Index)
		switch exp.Desc.Kind {
		case ExportKindFunction:
			if index >= len(target.Functions) {
				return nil, fmt.Errorf("unknown function for export")
			}
			target.Exports[name] = &ExportInstance{
				Kind:     exp.Desc.Kind,
				Function: target.Functions[index],
			}
		case ExportKindGlobal:
			if index >= len(target.Globals) {
				return nil, fmt.Errorf("unknown global for export")
			}
			target.Exports[name] = &ExportInstance{
				Kind:   exp.Desc.Kind,
				Global: target.Globals[exp.Desc.Index],
			}
		case ExportKindMemory:
			if index != 0 || target.Memory == nil {
				return nil, fmt.Errorf("unknown memory for export")
			}
			target.Exports[name] = &ExportInstance{
				Kind:   exp.Desc.Kind,
				Memory: target.Memory,
			}
		case ExportKindTable:
			if index >= len(target.Tables) {
				return nil, fmt.Errorf("unknown memory for export")
			}
			target.Exports[name] = &ExportInstance{
				Kind:  exp.Desc.Kind,
				Table: target.Tables[exp.Desc.Index],
			}
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

type BlockType = FunctionType

func analyzeFunction(
	module *Module, f *FunctionInstance,
	functionDeclarations []uint32,
	globalDeclarations []*GlobalType,
	memoryDeclarations []*MemoryType,
	tableDeclarations []*TableType,
) error {
	labelStack := []*FunctionInstanceBlock{
		{BlockType: f.Signature, StartAt: math.MaxUint64},
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
			switch Opcode(op) {
			case OpcodeLocalGet:
				inputLen := uint32(len(f.Signature.Params))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.get %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				if index < inputLen {
					valueTypeStack.push(f.Signature.Params[index])
				} else {
					valueTypeStack.push(f.LocalTypes[index-inputLen])
				}
			case OpcodeLocalSet:
				inputLen := uint32(len(f.Signature.Params))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.set %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.Signature.Params[index]
				} else {
					expType = f.LocalTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
			case OpcodeLocalTee:
				inputLen := uint32(len(f.Signature.Params))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.tee %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.Signature.Params[index]
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
			bt, num, err := ReadBlockType(module.TypeSection, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &FunctionInstanceBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeLoop {
			bt, num, err := ReadBlockType(module.TypeSection, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &FunctionInstanceBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
				IsLoop:         true,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeIf {
			bt, num, err := ReadBlockType(module.TypeSection, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &FunctionInstanceBlock{
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
			expTypes := f.Signature.Results
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

func ReadBlockType(types []*FunctionType, r io.Reader) (*BlockType, uint64, error) {
	raw, num, err := leb128.DecodeInt33AsInt64(r)
	if err != nil {
		return nil, 0, fmt.Errorf("decode int33: %w", err)
	}

	var ret *BlockType
	switch raw {
	case -64: // 0x40 in original byte = nil
		ret = &BlockType{}
	case -1: // 0x7f in original byte = i32
		ret = &BlockType{Results: []ValueType{ValueTypeI32}}
	case -2: // 0x7e in original byte = i64
		ret = &BlockType{Results: []ValueType{ValueTypeI64}}
	case -3: // 0x7d in original byte = f32
		ret = &BlockType{Results: []ValueType{ValueTypeF32}}
	case -4: // 0x7c in original byte = f64
		ret = &BlockType{Results: []ValueType{ValueTypeF64}}
	default:
		if raw < 0 || (raw >= int64(len(types))) {
			return nil, 0, fmt.Errorf("invalid block type: %d", raw)
		}
		ret = types[raw]
	}
	return ret, num, nil
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
	getSignature := func(p reflect.Type) (*FunctionType, error) {
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

	_, ok := m.Exports[funcName]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", funcName, moduleName)
	}

	sig, err := getSignature(fn.Type())
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	f := &FunctionInstance{
		Name:           fmt.Sprintf("%s.%s", moduleName, funcName),
		HostFunction:   &fn,
		Signature:      sig,
		ModuleInstance: m,
	}
	if err := s.engine.PreCompile([]*FunctionInstance{f}); err != nil {
		return fmt.Errorf("failed to precompile %s: %v", f.Name, err)
	}

	if err := s.engine.Compile(f); err != nil {
		return fmt.Errorf("failed to compile %s: %v", f.Name, err)
	}
	m.Exports[funcName] = &ExportInstance{Kind: ExportKindFunction, Function: f}
	s.Functions = append(s.Functions, f)
	return nil
}

func (s *Store) AddGlobal(moduleName, name string, value uint64, valueType ValueType, mutable bool) error {
	m := s.getModuleInstance(moduleName)

	_, ok := m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}
	g := &GlobalInstance{
		Val:  value,
		Type: &GlobalType{Mutable: mutable, ValType: valueType},
	}
	m.Exports[name] = &ExportInstance{Kind: ExportKindGlobal, Global: g}
	s.Globals = append(s.Globals, g)
	return nil
}

func (s *Store) AddTableInstance(moduleName, name string, min uint32, max *uint32) error {
	m := s.getModuleInstance(moduleName)

	_, ok := m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}

	table := &TableInstance{
		Table:    make([]*TableInstanceElm, min),
		Min:      min,
		Max:      max,
		ElemType: 0x70, // funcref
	}
	m.Exports[name] = &ExportInstance{Kind: ExportKindTable, Table: table}
	s.Tables = append(s.Tables, table)
	return nil
}

func (s *Store) AddMemoryInstance(moduleName, name string, min uint32, max *uint32) error {
	m := s.getModuleInstance(moduleName)

	_, ok := m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}

	memory := &MemoryInstance{
		Buffer: make([]byte, uint64(min)*PageSize),
		Min:    min,
		Max:    max,
	}
	m.Exports[name] = &ExportInstance{Kind: ExportKindMemory, Memory: memory}
	s.Memories = append(s.Memories, memory)
	return nil
}

// getModuleInstance returns an existing ModuleInstance if exists, or assigns a new one.
func (s *Store) getModuleInstance(name string) *ModuleInstance {
	m, ok := s.ModuleInstances[name]
	if !ok {
		m = &ModuleInstance{Exports: map[string]*ExportInstance{}}
		s.ModuleInstances[name] = m
	}
	return m
}
