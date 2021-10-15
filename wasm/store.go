package wasm

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/mathetake/gasm/wasm/leb128"
)

type (
	Store struct {
		ModuleInstances map[string]*ModuleInstance

		Functions []FuncInstance
		Globals   []*GlobalInstance
		Memories  []*MemoryInstance
		Tables    []*TableInstance
	}

	ModuleInstance struct {
		Exports       map[string]*ExportInstance
		GlobalsAddrs  []int
		FunctionAddrs []int
		MemoryAddrs   []int
		TableAddrs    []int

		Types []*FunctionType
	}

	ExportInstance struct {
		Kind byte
		Addr int
	}

	FuncInstance interface {
		Call(vm *VirtualMachine)
		FunctionType() *FunctionType
	}

	GlobalInstance struct {
		Type *GlobalType
		Val  uint64
	}

	TableInstance struct {
		Table    []*uint32
		Min      uint32
		Max      *uint32
		ElemType byte
	}

	MemoryInstance struct {
		Memory []byte
		Min    uint32
		Max    *uint32
	}
)

func NewStore() *Store {
	return &Store{ModuleInstances: map[string]*ModuleInstance{}}
}

func (s *Store) Instantiate(module *Module, name string) (*ModuleInstance, error) {
	instance := &ModuleInstance{Types: module.TypeSection}
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
	// Check the start function is valid.
	if module.StartSection != nil {
		index := *module.StartSection
		if int(index) >= len(instance.FunctionAddrs) {
			return nil, fmt.Errorf("invalid start function index: %d", index)
		}
		signature := s.Functions[instance.FunctionAddrs[index]].FunctionType()
		if len(signature.InputTypes) != 0 || len(signature.ReturnTypes) != 0 {
			return nil, fmt.Errorf("start function must have the empty signature")
		}
	}
	// Now we are safe to finalize the state.
	rollbackFuncs = nil
	return instance, nil
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
	case 0x00: // function
		if err := s.applyFunctionImport(target, is.Desc.TypeIndexPtr, e); err != nil {
			return fmt.Errorf("applyFunctionImport: %w", err)
		}
	case 0x01: // table
		if err := s.applyTableImport(target, is.Desc.TableTypePtr, e); err != nil {
			return fmt.Errorf("applyTableImport: %w", err)
		}
	case 0x02: // mem
		if err := s.applyMemoryImport(target, is.Desc.MemTypePtr, e); err != nil {
			return fmt.Errorf("applyMemoryImport: %w", err)
		}
	case 0x03: // global
		if err := s.applyGlobalImport(target, is.Desc.GlobalTypePtr, e); err != nil {
			return fmt.Errorf("applyGlobalImport: %w", err)
		}
	default:
		return fmt.Errorf("invalid kind of import: %#x", is.Desc.Kind)
	}

	return nil
}

func (s *Store) applyFunctionImport(target *ModuleInstance, typeIndexPtr *uint32, externModuleExportIsntance *ExportInstance) error {
	if typeIndexPtr == nil {
		return fmt.Errorf("type index is invalid")
	}
	f := s.Functions[externModuleExportIsntance.Addr]
	typeIndex := *typeIndexPtr
	if int(typeIndex) >= len(target.Types) {
		return fmt.Errorf("unknown type for function import")
	}
	iSig := target.Types[typeIndex]
	if !hasSameSignature(iSig.ReturnTypes, f.FunctionType().ReturnTypes) {
		return fmt.Errorf("return signature mimatch: %#x != %#x", iSig.ReturnTypes, f.FunctionType().ReturnTypes)
	} else if !hasSameSignature(iSig.InputTypes, f.FunctionType().InputTypes) {
		return fmt.Errorf("input signature mimatch: %#x != %#x", iSig.InputTypes, f.FunctionType().InputTypes)
	}
	target.FunctionAddrs = append(target.FunctionAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyTableImport(target *ModuleInstance, tableTypePtr *TableType, externModuleExportIsntance *ExportInstance) error {
	t := s.Tables[externModuleExportIsntance.Addr]
	if tableTypePtr == nil {
		return fmt.Errorf("table type is invalid")
	}
	if t.ElemType != tableTypePtr.ElemType {
		return fmt.Errorf("incompatible table imports: element type mismatch")
	}
	if t.Min < tableTypePtr.Limit.Min {
		return fmt.Errorf("incompatible table imports: minimum size mismatch")
	}

	if tableTypePtr.Limit.Max != nil {
		if t.Max == nil {
			return fmt.Errorf("incompatible table imports: maximum size mismatch")
		} else if *t.Max > *tableTypePtr.Limit.Max {
			return fmt.Errorf("incompatible table imports: maximum size mismatch")
		}
	}
	target.TableAddrs = append(target.TableAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyMemoryImport(target *ModuleInstance, memoryTypePtr *MemoryType, externModuleExportIsntance *ExportInstance) error {
	if memoryTypePtr == nil {
		return fmt.Errorf("memory type is invalid")
	}
	t := s.Memories[externModuleExportIsntance.Addr]
	if t.Min < memoryTypePtr.Min {
		return fmt.Errorf("incompatible memory imports: minimum size mismatch")
	}
	if memoryTypePtr.Max != nil {
		if t.Max == nil {
			return fmt.Errorf("incompatible memory imports: maximum size mismatch")
		} else if *t.Max > *memoryTypePtr.Max {
			return fmt.Errorf("incompatible memory imports: maximum size mismatch")
		}
	}
	target.MemoryAddrs = append(target.MemoryAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyGlobalImport(target *ModuleInstance, globalTypePtr *GlobalType, externModuleExportIsntance *ExportInstance) error {
	if globalTypePtr == nil {
		return fmt.Errorf("global type is invalid")
	}
	t := s.Globals[externModuleExportIsntance.Addr]
	if globalTypePtr.Mutable != t.Type.Mutable {
		return fmt.Errorf("incompatible global import: mutability mismatch")
	} else if globalTypePtr.ValType != t.Type.ValType {
		return fmt.Errorf("incompatible global import: value type mismatch")
	}
	target.GlobalsAddrs = append(target.GlobalsAddrs, externModuleExportIsntance.Addr)
	return nil
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
		target.GlobalsAddrs = append(target.GlobalsAddrs, len(s.Globals))
		s.Globals = append(s.Globals, &GlobalInstance{
			Type: gs.Type,
			Val:  gv,
		})
	}
	return rollbackFuncs, nil
}

func (s *Store) buildFunctionInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	prevLen := len(s.Functions)
	rollbackFuncs = append(rollbackFuncs, func() {
		s.Functions = s.Functions[:prevLen]
	})
	var functionDeclarations []uint32
	var globalDecalarations []*GlobalType
	var memoryDeclarations []*MemoryType
	var tableDeclarations []*TableType
	for _, imp := range module.ImportSection {
		switch imp.Desc.Kind {
		case ImportKindFunction:
			functionDeclarations = append(functionDeclarations, *imp.Desc.TypeIndexPtr)
		case ImportKindGlobal:
			globalDecalarations = append(globalDecalarations, imp.Desc.GlobalTypePtr)
		case ImportKindMemory:
			memoryDeclarations = append(memoryDeclarations, imp.Desc.MemTypePtr)
		case ImportKindTable:
			tableDeclarations = append(tableDeclarations, imp.Desc.TableTypePtr)
		}
	}
	functionDeclarations = append(functionDeclarations, module.FunctionSection...)
	for _, g := range module.GlobalSection {
		globalDecalarations = append(globalDecalarations, g.Type)
	}
	memoryDeclarations = append(memoryDeclarations, module.MemorySection...)
	tableDeclarations = append(tableDeclarations, module.TableSection...)

	for codeIndex, typeIndex := range module.FunctionSection {
		if typeIndex >= uint32(len(module.TypeSection)) {
			return rollbackFuncs, fmt.Errorf("function type index out of range")
		} else if codeIndex >= len(module.CodeSection) {
			return rollbackFuncs, fmt.Errorf("code index out of range")
		}

		f := &NativeFunction{
			Signature:      module.TypeSection[typeIndex],
			Body:           module.CodeSection[codeIndex].Body,
			NumLocals:      module.CodeSection[codeIndex].NumLocals,
			LocalTypes:     module.CodeSection[codeIndex].LocalTypes,
			ModuleInstance: target,
			Blocks:         map[uint64]*NativeFunctionBlock{},
		}

		err := analyzeFunction(
			module, f, functionDeclarations, globalDecalarations,
			memoryDeclarations, tableDeclarations,
		)
		if err != nil {
			return rollbackFuncs, fmt.Errorf("invalid function at index %d/%d: %v", codeIndex, len(module.FunctionSection), err)
		}
		target.FunctionAddrs = append(target.FunctionAddrs, len(s.Functions))
		s.Functions = append(s.Functions, f)
	}
	return rollbackFuncs, nil
}

func (s *Store) buildMemoryInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	// Allocate memory instances.
	for _, memSec := range module.MemorySection {
		memInst := &MemoryInstance{
			Memory: make([]byte, uint64(memSec.Min)*PageSize),
			Min:    memSec.Min,
			Max:    memSec.Max,
		}
		target.MemoryAddrs = append(target.MemoryAddrs, len(s.Memories))
		s.Memories = append(s.Memories, memInst)
	}

	// Initialize the memory instance according to the Data section.
	for _, d := range module.DataSection {
		if d.MemoryIndex >= uint32(len(target.MemoryAddrs)) {
			return rollbackFuncs, fmt.Errorf("index out of range of index space")
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

		memoryInst := s.Memories[target.MemoryAddrs[d.MemoryIndex]]
		if size > uint64(len(memoryInst.Memory)) {
			return rollbackFuncs, fmt.Errorf("out of bounds memory access")
		}
		// Setup the rollback function before mutating the acutal memory.
		original := make([]byte, len(d.Init))
		copy(original, memoryInst.Memory[offset:])
		rollbackFuncs = append(rollbackFuncs, func() {
			copy(memoryInst.Memory[offset:], original)
		})
		copy(memoryInst.Memory[offset:], d.Init)
	}
	if len(target.MemoryAddrs) > 1 {
		return rollbackFuncs, fmt.Errorf("multiple memories not supported")
	}
	return rollbackFuncs, nil
}

func (s *Store) buildTableInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	// Allocate table instances.
	for _, tableSeg := range module.TableSection {
		tableInst := &TableInstance{
			Table:    make([]*uint32, tableSeg.Limit.Min),
			Min:      tableSeg.Limit.Min,
			Max:      tableSeg.Limit.Max,
			ElemType: tableSeg.ElemType,
		}
		target.TableAddrs = append(target.TableAddrs, len(s.Tables))
		s.Tables = append(s.Tables, tableInst)
	}

	for _, elem := range module.ElementSection {
		if elem.TableIndex >= uint32(len(target.TableAddrs)) {
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

		tableInst := s.Tables[target.TableAddrs[elem.TableIndex]]
		if size > len(tableInst.Table) {
			return rollbackFuncs, fmt.Errorf("out of bounds table access %d > %v", size, tableInst.Min)
		}
		for i := range elem.Init {
			i := i
			elm := elem.Init[i]
			if elm >= uint32(len(target.FunctionAddrs)) {
				return rollbackFuncs, fmt.Errorf("unknown function specified by element")
			}
			// Setup the rollback function before mutating the table instance.
			pos := i + offset
			original := tableInst.Table[pos]
			rollbackFuncs = append(rollbackFuncs, func() {
				tableInst.Table[pos] = original
			})
			addr := uint32(target.FunctionAddrs[elm])
			tableInst.Table[pos] = &addr
		}
	}
	if len(target.TableAddrs) > 1 {
		return rollbackFuncs, fmt.Errorf("multiple tables not supported")
	}
	return rollbackFuncs, nil
}

func (s *Store) buildExportInstances(module *Module, target *ModuleInstance) (rollbackFuncs []func(), err error) {
	target.Exports = make(map[string]*ExportInstance, len(module.ExportSection))
	for name, exp := range module.ExportSection {
		var addr int
		index := int(exp.Desc.Index)
		switch exp.Desc.Kind {
		case ExportKindFunction:
			if index >= len(target.FunctionAddrs) {
				return nil, fmt.Errorf("unknown function for export")
			}
			addr = target.FunctionAddrs[index]
		case ExportKindGlobal:
			if index >= len(target.GlobalsAddrs) {
				return nil, fmt.Errorf("unknown global for export")
			}
			addr = target.GlobalsAddrs[exp.Desc.Index]
		case ExportKindMemory:
			if index >= len(target.MemoryAddrs) {
				return nil, fmt.Errorf("unknown memory for export")
			}
			addr = target.MemoryAddrs[exp.Desc.Index]
		case ExportKindTable:
			if index >= len(target.TableAddrs) {
				return nil, fmt.Errorf("unknown memory for export")
			}
			addr = target.TableAddrs[exp.Desc.Index]
		}
		target.Exports[name] = &ExportInstance{
			Kind: exp.Desc.Kind,
			Addr: addr,
		}
	}
	return
}

type valueTypeStack struct {
	stack       []ValueType
	stackLimits []int
}

const (
	valueTypeUnknown = ValueType(0xFE)
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
	module *Module, f *NativeFunction,
	functionDeclarations []uint32,
	globalDeclarations []*GlobalType,
	memoryDeclarations []*MemoryType,
	tableDeclarations []*TableType,
) error {
	labelStack := []*NativeFunctionBlock{
		{BlockType: f.Signature, StartAt: math.MaxUint64},
	}
	valueTypeStack := &valueTypeStack{}
	for pc := uint64(0); pc < uint64(len(f.Body)); pc++ {
		rawOc := f.Body[pc]
		if 0x28 <= rawOc && rawOc <= 0x3e { // memory load,store
			if len(memoryDeclarations) == 0 {
				return fmt.Errorf("unknown memory access")
			}
			pc++
			align, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read memory align: %v", err)
			}
			switch OptCode(rawOc) {
			case OptCodeI32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeF32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeI32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeF32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeF64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeI64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeF64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI32Load8s:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32Load8u:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64Load8s, OptCodeI64Load8u:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI32Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI64Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI32Load16s, OptCodeI32Load16u:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64Load16s, OptCodeI64Load16u:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI32Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI64Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OptCodeI64Load32s, OptCodeI64Load32u:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI64Store32:
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
		} else if 0x3f <= rawOc && rawOc <= 0x40 { // memory grow,size
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
			switch OptCode(rawOc) {
			case OptCodeMemoryGrow:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeMemorySize:
				valueTypeStack.push(ValueTypeI32)
			}
			pc += num - 1
		} else if 0x41 <= rawOc && rawOc <= 0x44 { // const instructions
			pc++
			switch OptCode(rawOc) {
			case OptCodeI32Const:
				_, num, err := leb128.DecodeInt32(bytes.NewBuffer(f.Body[pc:]))
				if err != nil {
					return fmt.Errorf("read i32 immediate: %s", err)
				}
				pc += num - 1
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64Const:
				_, num, err := leb128.DecodeInt64(bytes.NewBuffer(f.Body[pc:]))
				if err != nil {
					return fmt.Errorf("read i64 immediate: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
				pc += num - 1
			case OptCodeF32Const:
				valueTypeStack.push(ValueTypeF32)
				pc += 3
			case OptCodeF64Const:
				valueTypeStack.push(ValueTypeF64)
				pc += 7
			}
		} else if 0x20 <= rawOc && rawOc <= 0x24 { // variable instructions
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewBuffer(f.Body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			switch OptCode(rawOc) {
			case OptCodeLocalGet:
				inputLen := uint32(len(f.Signature.InputTypes))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.get %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				if index < inputLen {
					valueTypeStack.push(f.Signature.InputTypes[index])
				} else {
					valueTypeStack.push(f.LocalTypes[index-inputLen])
				}
			case OptCodeLocalSet:
				inputLen := uint32(len(f.Signature.InputTypes))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.set %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.Signature.InputTypes[index]
				} else {
					expType = f.LocalTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
			case OptCodeLocalTee:
				inputLen := uint32(len(f.Signature.InputTypes))
				if l := f.NumLocals + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.tee %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = f.Signature.InputTypes[index]
				} else {
					expType = f.LocalTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
				valueTypeStack.push(expType)
			case OptCodeGlobalGet:
				if index >= uint32(len(globalDeclarations)) {
					return fmt.Errorf("invalid global index")
				}
				valueTypeStack.push(globalDeclarations[index].ValType)
			case OptCodeGlobalSet:
				if index >= uint32(len(globalDeclarations)) {
					return fmt.Errorf("invalid global index")
				} else if !globalDeclarations[index].Mutable {
					return fmt.Errorf("globa.set on immutable global type")
				} else if err := valueTypeStack.popAndVerifyType(
					globalDeclarations[index].ValType); err != nil {
					return err
				}
			}
		} else if rawOc == 0x0c { // br
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
			targetResultType := target.BlockType.ReturnTypes
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
		} else if rawOc == 0x0d { // br_if
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
			targetResultType := target.BlockType.ReturnTypes
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
		} else if rawOc == 0x0e { // br_table
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
			expType := lnLabel.BlockType.ReturnTypes
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
				expType2 := label.BlockType.ReturnTypes
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
		} else if rawOc == 0x10 { // call
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
			for i := 0; i < len(funcType.InputTypes); i++ {
				if err := valueTypeStack.popAndVerifyType(funcType.InputTypes[len(funcType.InputTypes)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call operation input type")
				}
			}
			for _, exp := range funcType.ReturnTypes {
				valueTypeStack.push(exp)
			}
		} else if rawOc == 0x11 { // call_indirect
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
			for i := 0; i < len(funcType.InputTypes); i++ {
				if err := valueTypeStack.popAndVerifyType(funcType.InputTypes[len(funcType.InputTypes)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call_indirect operation input type")
				}
			}
			for _, exp := range funcType.ReturnTypes {
				valueTypeStack.push(exp)
			}
		} else if 0x45 <= rawOc && rawOc <= 0xbf { // numeric instructions
			switch OptCode(rawOc) {
			case OptCodeI32eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32eq, OptCodeI32ne, OptCodeI32lts,
				OptCodeI32ltu, OptCodeI32gts, OptCodeI32gtu, OptCodeI32les,
				OptCodeI32leu, OptCodeI32ges, OptCodeI32geu:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64eq, OptCodeI64ne, OptCodeI64lts,
				OptCodeI64ltu, OptCodeI64gts, OptCodeI64gtu,
				OptCodeI64les, OptCodeI64leu, OptCodeI64ges, OptCodeI64geu:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeF32eq, OptCodeF32ne, OptCodeF32lt, OptCodeF32gt, OptCodeF32le, OptCodeF32ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeF64eq, OptCodeF64ne, OptCodeF64lt, OptCodeF64gt, OptCodeF64le, OptCodeF64ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32clz, OptCodeI32ctz, OptCodeI32popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32add, OptCodeI32sub, OptCodeI32mul, OptCodeI32divs,
				OptCodeI32divu, OptCodeI32rems, OptCodeI32remu, OptCodeI32and,
				OptCodeI32or, OptCodeI32xor, OptCodeI32shl, OptCodeI32shrs,
				OptCodeI32shru, OptCodeI32rotl, OptCodeI32rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64clz, OptCodeI64ctz, OptCodeI64popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI64add, OptCodeI64sub, OptCodeI64mul, OptCodeI64divs,
				OptCodeI64divu, OptCodeI64rems, OptCodeI64remu, OptCodeI64and,
				OptCodeI64or, OptCodeI64xor, OptCodeI64shl, OptCodeI64shrs,
				OptCodeI64shru, OptCodeI64rotl, OptCodeI64rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeF32abs, OptCodeF32neg, OptCodeF32ceil,
				OptCodeF32floor, OptCodeF32trunc, OptCodeF32nearest,
				OptCodeF32sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF32add, OptCodeF32sub, OptCodeF32mul,
				OptCodeF32div, OptCodeF32min, OptCodeF32max,
				OptCodeF32copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF64abs, OptCodeF64neg, OptCodeF64ceil,
				OptCodeF64floor, OptCodeF64trunc, OptCodeF64nearest,
				OptCodeF64sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeF64add, OptCodeF64sub, OptCodeF64mul,
				OptCodeF64div, OptCodeF64min, OptCodeF64max,
				OptCodeF64copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", rawOc, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeI32wrapI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.wrap_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32truncf32s, OptCodeI32truncf32u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI32truncf64s, OptCodeI32truncf64u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64Extendi32s, OptCodeI64Extendi32u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI64TruncF32s, OptCodeI64TruncF32u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeI64Truncf64s, OptCodeI64Truncf64u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeF32Converti32s, OptCodeF32Converti32u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF32Converti64s, OptCodeF32Converti64u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF32Demotef64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.demote_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF64Converti32s, OptCodeF64Converti32u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeF64Converti64s, OptCodeF64Converti64u:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", rawOc, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeF64Promotef32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.promote_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OptCodeI32reinterpretf32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.reinterpret_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OptCodeI64reinterpretf64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.reinterpret_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OptCodeF32reinterpreti32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.reinterpret_i32: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OptCodeF64reinterpreti64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.reinterpret_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			default:
				return fmt.Errorf("invalid numeric instruction 0x%x", rawOc)
			}
		} else if rawOc == 0x02 { // Block
			bt, num, err := readBlockType(module, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &NativeFunctionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if rawOc == 0x03 { // Loop
			bt, num, err := readBlockType(module, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &NativeFunctionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
				IsLoop:         true,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if rawOc == 0x04 { // If
			bt, num, err := readBlockType(module, bytes.NewBuffer(f.Body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			labelStack = append(labelStack, &NativeFunctionBlock{
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
		} else if rawOc == 0x05 { // Else
			bl := labelStack[len(labelStack)-1]
			bl.ElseAt = pc
			// Check the type soundness of the instructions *before*　 entering this Eles Op.
			if err := valueTypeStack.popResults(bl.BlockType.ReturnTypes, true); err != nil {
				return fmt.Errorf("invalid instruction results in then instructions")
			}
			// Before entring instructions inside else, we pop all the values pushed by
			// then block.
			valueTypeStack.resetAtStackLimit()
		} else if rawOc == 0x0b { // End
			bl := labelStack[len(labelStack)-1]
			bl.EndAt = pc
			labelStack = labelStack[:len(labelStack)-1]
			f.Blocks[bl.StartAt] = bl
			if bl.IsIf && bl.ElseAt <= bl.StartAt {
				if len(bl.BlockType.ReturnTypes) > 0 {
					return fmt.Errorf("type mismatch between then and else blocks.")
				}
				// To handle if block without else properly,
				// we set ElseAt to EndAt-1 so we can just skip else.
				bl.ElseAt = bl.EndAt - 1
			}
			// Check type soundness.
			if err := valueTypeStack.popResults(bl.BlockType.ReturnTypes, true); err != nil {
				return fmt.Errorf("invalid instruction results at end instruction; expected %v: %v", bl.BlockType.ReturnTypes, err)
			}
			// Put the result types at the end after resetting at the stack limit
			// since we might have Any type between the limit and the current top.
			valueTypeStack.resetAtStackLimit()
			for _, exp := range bl.BlockType.ReturnTypes {
				valueTypeStack.push(exp)
			}
			// We exit if/loop/block, so reset the constraints on the stack manipulation
			// on values previously pushed by outer blocks.
			valueTypeStack.popStackLimit()
		} else if rawOc == 0x0f { // Return
			expTypes := f.Signature.ReturnTypes
			for i := 0; i < len(expTypes); i++ {
				if err := valueTypeStack.popAndVerifyType(expTypes[len(expTypes)-1-i]); err != nil {
					return fmt.Errorf("return type mismatch on return: %v; want %v", err, expTypes)
				}
			}
			// return instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if rawOc == 0x1a { // Drop
			_, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid drop: %v", err)
			}
		} else if rawOc == 0x1b { // Select
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
		} else if rawOc == 0x00 { // unreachable
			// unreachable instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if rawOc == 0x01 { // Nop
		} else {
			return fmt.Errorf("invalid instruction 0x%x", rawOc)
		}
	}

	if len(labelStack) > 0 {
		return fmt.Errorf("ill-nested block exists")
	}

	return nil
}

func readBlockType(module *Module, r io.Reader) (*BlockType, uint64, error) {
	raw, num, err := leb128.DecodeInt33AsInt64(r)
	if err != nil {
		return nil, 0, fmt.Errorf("decode int33: %w", err)
	}

	var ret *BlockType
	switch raw {
	case -64: // 0x40 in original byte = nil
		ret = &BlockType{}
	case -1: // 0x7f in original byte = i32
		ret = &BlockType{ReturnTypes: []ValueType{ValueTypeI32}}
	case -2: // 0x7e in original byte = i64
		ret = &BlockType{ReturnTypes: []ValueType{ValueTypeI64}}
	case -3: // 0x7d in original byte = f32
		ret = &BlockType{ReturnTypes: []ValueType{ValueTypeF32}}
	case -4: // 0x7c in original byte = f64
		ret = &BlockType{ReturnTypes: []ValueType{ValueTypeF64}}
	default:
		if raw < 0 || (raw >= int64(len(module.TypeSection))) {
			return nil, 0, fmt.Errorf("invalid block type: %d", raw)
		}
		ret = module.TypeSection[raw]
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
			return nil, fmt.Errorf("host function must accept *VirtualMachine as the first param")
		}
		in := make([]ValueType, p.NumIn()-1)
		for i := range in {
			in[i], err = getTypeOf(p.In(i + 1).Kind())
			if err != nil {
				return nil, err
			}
		}

		out := make([]ValueType, p.NumOut())
		for i := range out {
			out[i], err = getTypeOf(p.Out(i).Kind())
			if err != nil {
				return nil, err
			}
		}
		return &FunctionType{InputTypes: in, ReturnTypes: out}, nil
	}

	m, ok := s.ModuleInstances[moduleName]
	if !ok {
		m = &ModuleInstance{Exports: map[string]*ExportInstance{}}
		s.ModuleInstances[moduleName] = m
	}

	_, ok = m.Exports[funcName]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", funcName, moduleName)
	}

	sig, err := getSignature(fn.Type())
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	m.Exports[funcName] = &ExportInstance{
		Kind: ExportKindFunction,
		Addr: len(s.Functions),
	}
	s.Functions = append(s.Functions, &HostFunction{
		Name:      fmt.Sprintf("%s.%s", moduleName, funcName),
		Function:  fn,
		Signature: sig,
	})
	return nil
}

func (s *Store) AddGlobal(moduleName, name string, value uint64, valueType ValueType, mutable bool) error {
	m, ok := s.ModuleInstances[moduleName]
	if !ok {
		m = &ModuleInstance{}
		s.ModuleInstances[moduleName] = m
	}

	_, ok = m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}
	m.Exports[name] = &ExportInstance{
		Kind: ExportKindGlobal,
		Addr: len(s.Globals),
	}
	s.Globals = append(s.Globals, &GlobalInstance{
		Val:  value,
		Type: &GlobalType{Mutable: mutable, ValType: valueType},
	})
	return nil
}

func (s *Store) AddTableInstance(moduleName, name string, min uint32, max *uint32) error {
	m, ok := s.ModuleInstances[moduleName]
	if !ok {
		m = &ModuleInstance{}
		s.ModuleInstances[moduleName] = m
	}

	_, ok = m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}

	m.Exports[name] = &ExportInstance{
		Kind: ExportKindTable,
		Addr: len(s.Tables),
	}
	s.Tables = append(s.Tables, &TableInstance{
		Table:    make([]*uint32, min),
		Min:      min,
		Max:      max,
		ElemType: 0x70, // funcref
	})
	return nil
}

func (s *Store) AddMemoryInstance(moduleName, name string, min uint32, max *uint32) error {
	m, ok := s.ModuleInstances[moduleName]
	if !ok {
		m = &ModuleInstance{}
		s.ModuleInstances[moduleName] = m
	}

	_, ok = m.Exports[name]
	if ok {
		return fmt.Errorf("name %s already exists in module %s", name, moduleName)
	}

	m.Exports[name] = &ExportInstance{
		Kind: ExportKindMemory,
		Addr: len(s.Memories),
	}
	s.Memories = append(s.Memories, &MemoryInstance{
		Memory: make([]byte, uint64(min)*PageSize),
		Min:    min,
		Max:    max,
	})
	return nil
}
