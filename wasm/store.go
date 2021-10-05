package wasm

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"reflect"

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
	ret := &ModuleInstance{Types: module.SecTypes}
	s.ModuleInstances[name] = ret

	if err := s.resolveImports(module, ret); err != nil {
		return nil, fmt.Errorf("resolve imports: %w", err)
	}
	if err := s.buildGlobalInstances(module, ret); err != nil {
		return nil, fmt.Errorf("globals: %w", err)
	}
	if err := s.buildFunctionInstances(module, ret); err != nil {
		return nil, fmt.Errorf("functions: %w", err)
	}
	if err := s.buildTableInstances(module, ret); err != nil {
		return nil, fmt.Errorf("tables: %w", err)
	}
	if err := s.buildMemoryInstances(module, ret); err != nil {
		return nil, fmt.Errorf("memories: %w", err)
	}
	if err := s.buildExportInstances(module, ret); err != nil {
		return nil, fmt.Errorf("exports: %w", err)
	}

	// TODO: Execute start func
	return ret, nil
}

func (s *Store) resolveImports(module *Module, target *ModuleInstance) error {
	for _, is := range module.SecImports {
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
		if err := s.applyFunctionImport(target, is, e); err != nil {
			return fmt.Errorf("applyFunctionImport: %w", err)
		}
	case 0x01: // table
		if err := s.applyTableImport(target, e); err != nil {
			return fmt.Errorf("applyTableImport: %w", err)
		}
	case 0x02: // mem
		if err := s.applyMemoryImport(target, e); err != nil {
			return fmt.Errorf("applyMemoryImport: %w", err)
		}
	case 0x03: // global
		if err := s.applyGlobalImport(target, e); err != nil {
			return fmt.Errorf("applyGlobalImport: %w", err)
		}
	default:
		return fmt.Errorf("invalid kind of import: %#x", is.Desc.Kind)
	}

	return nil
}

func (s *Store) applyFunctionImport(target *ModuleInstance, is *ImportSegment, externModuleExportIsntance *ExportInstance) error {
	if is.Desc.TypeIndexPtr == nil {
		return fmt.Errorf("is.Desc.TypeIndexPtr is nill")
	}

	f := s.Functions[externModuleExportIsntance.Addr]
	iSig := target.Types[*is.Desc.TypeIndexPtr]
	if !hasSameSignature(iSig.ReturnTypes, f.FunctionType().ReturnTypes) {
		return fmt.Errorf("return signature mimatch: %#x != %#x", iSig.ReturnTypes, f.FunctionType().ReturnTypes)
	} else if !hasSameSignature(iSig.InputTypes, f.FunctionType().InputTypes) {
		return fmt.Errorf("input signature mimatch: %#x != %#x", iSig.InputTypes, f.FunctionType().InputTypes)
	}
	target.FunctionAddrs = append(target.FunctionAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyTableImport(target *ModuleInstance, externModuleExportIsntance *ExportInstance) error {
	// TODO: verify the limit compatibility.
	// TODO: verify the type compatibility.
	target.TableAddrs = append(target.TableAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyMemoryImport(target *ModuleInstance, externModuleExportIsntance *ExportInstance) error {
	// TODO: verify the limit compatibility.
	target.MemoryAddrs = append(target.MemoryAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) applyGlobalImport(target *ModuleInstance, externModuleExportIsntance *ExportInstance) error {
	// TODO: verify the type compatibility.
	target.GlobalsAddrs = append(target.GlobalsAddrs, externModuleExportIsntance.Addr)
	return nil
}

func (s *Store) buildGlobalInstances(module *Module, target *ModuleInstance) error {
	for _, gs := range module.SecGlobals {
		raw, err := s.executeConstExpression(target, gs.Init)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
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
	return nil
}

func (s *Store) buildFunctionInstances(module *Module, target *ModuleInstance) error {
	for codeIndex, typeIndex := range module.SecFunctions {
		if typeIndex >= uint32(len(module.SecTypes)) {
			return fmt.Errorf("function type index out of range")
		} else if codeIndex >= len(module.SecCodes) {
			return fmt.Errorf("code index out of range")
		}

		f := &NativeFunction{
			Signature:      module.SecTypes[typeIndex],
			Body:           module.SecCodes[codeIndex].Body,
			NumLocal:       module.SecCodes[codeIndex].NumLocals,
			ModuleInstance: target,
		}

		blocks, err := parseBlocks(module, f.Body)
		if err != nil {
			return fmt.Errorf("parse blocks: %w", err)
		}
		f.Blocks = blocks
		target.FunctionAddrs = append(target.FunctionAddrs, len(s.Functions))
		s.Functions = append(s.Functions, f)
	}
	return nil
}

func (s *Store) buildMemoryInstances(module *Module, target *ModuleInstance) error {
	// Allocate memory instances.
	for _, memSec := range module.SecMemory {
		memInst := &MemoryInstance{
			Memory: make([]byte, memSec.Min*vmPageSize),
			Min:    memSec.Min,
			Max:    memSec.Max,
		}
		target.MemoryAddrs = append(target.MemoryAddrs, len(s.Memories))
		s.Memories = append(s.Memories, memInst)
	}

	// Initialize the memory instance according to the Data section.
	for _, d := range module.SecData {
		if d.MemoryIndex >= uint32(len(target.MemoryAddrs)) {
			return fmt.Errorf("index out of range of index space")
		}

		rawOffset, err := s.executeConstExpression(target, d.OffsetExpression)
		if err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}

		offset, ok := rawOffset.(int32)
		if !ok {
			return fmt.Errorf("offset is not int32 but %T", rawOffset)
		}

		size := int(offset) + len(d.Init)
		max := uint32(math.MaxUint32)
		if int(d.MemoryIndex) < len(module.SecMemory) && module.SecMemory[d.MemoryIndex].Max != nil {
			max = *module.SecMemory[d.MemoryIndex].Max
		}
		if uint32(size) > max*vmPageSize {
			return fmt.Errorf("memory size out of limit %d * 64Ki", int(*(module.SecMemory[d.MemoryIndex].Max)))
		}

		memoryInst := s.Memories[target.MemoryAddrs[d.MemoryIndex]]
		if size > len(memoryInst.Memory) {
			next := make([]byte, size)
			copy(next, memoryInst.Memory)
			copy(next[offset:], d.Init)
			memoryInst.Memory = next
		} else {
			copy(memoryInst.Memory[offset:], d.Init)
		}
	}
	return nil
}

func (s *Store) buildTableInstances(module *Module, target *ModuleInstance) error {
	// Allocate table instances.
	for _, tableSeg := range module.SecTables {
		tableInst := &TableInstance{
			Table:    make([]*uint32, tableSeg.Limit.Min),
			Min:      tableSeg.Limit.Min,
			Max:      tableSeg.Limit.Max,
			ElemType: tableSeg.ElemType,
		}
		target.TableAddrs = append(target.TableAddrs, len(s.Tables))
		s.Tables = append(s.Tables, tableInst)
	}

	// Initialize the table elements according to the Elements section.
	for _, elem := range module.SecElements {
		if elem.TableIndex >= uint32(len(target.TableAddrs)) {
			return fmt.Errorf("index out of range of index space")
		}

		rawOffset, err := s.executeConstExpression(target, elem.OffsetExpr)
		if err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}

		offset32, ok := rawOffset.(int32)
		if !ok {
			return fmt.Errorf("offset is not int32 but %T", rawOffset)
		}

		offset := int(offset32)
		size := offset + len(elem.Init)

		max := uint32(math.MaxUint32)
		if int(elem.TableIndex) < len(module.SecTables) && module.SecTables[elem.TableIndex].Limit.Max != nil {
			max = *module.SecTables[elem.TableIndex].Limit.Max
		}

		if size > int(max) {
			return fmt.Errorf("table size out of limit of %d", max)
		}

		tableInst := s.Tables[target.TableAddrs[elem.TableIndex]]
		if size > len(tableInst.Table) {
			next := make([]*uint32, size)
			copy(next, tableInst.Table)
			for i := range elem.Init {
				addr := uint32(target.FunctionAddrs[elem.Init[i]])
				next[i+offset] = &addr
			}
			tableInst.Table = next
		} else {
			for i := range elem.Init {
				addr := uint32(target.FunctionAddrs[elem.Init[i]])
				tableInst.Table[i+offset] = &addr
			}
		}
	}
	return nil
}

func (s *Store) buildExportInstances(module *Module, target *ModuleInstance) error {
	target.Exports = make(map[string]*ExportInstance, len(module.SecExports))
	for name, exp := range module.SecExports {
		var addr int
		switch exp.Desc.Kind {
		case ExportKindFunction:
			addr = target.FunctionAddrs[exp.Desc.Index]
		case ExportKindGlobal:
			addr = target.GlobalsAddrs[exp.Desc.Index]
		case ExportKindMemory:
			addr = target.MemoryAddrs[addr]
		case ExportKindTable:
			addr = target.TableAddrs[addr]
		}
		target.Exports[name] = &ExportInstance{
			Kind: exp.Desc.Kind,
			Addr: addr,
		}
	}
	return nil
}

type BlockType = FunctionType

func parseBlocks(module *Module, body []byte) (map[uint64]*NativeFunctionBlock, error) {
	ret := map[uint64]*NativeFunctionBlock{}
	stack := make([]*NativeFunctionBlock, 0)
	for pc := uint64(0); pc < uint64(len(body)); pc++ {
		rawOc := body[pc]
		if 0x28 <= rawOc && rawOc <= 0x3e { // memory load,store
			pc++
			// align
			_, num, err := leb128.DecodeUint32(bytes.NewBuffer(body[pc:]))
			if err != nil {
				return nil, fmt.Errorf("read memory align: %w", err)
			}
			pc += num
			// offset
			_, num, err = leb128.DecodeUint32(bytes.NewBuffer(body[pc:]))
			if err != nil {
				return nil, fmt.Errorf("read memory offset: %w", err)
			}
			pc += num - 1
			continue
		} else if 0x41 <= rawOc && rawOc <= 0x44 { // const instructions
			pc++
			switch OptCode(rawOc) {
			case OptCodeI32Const:
				_, num, err := leb128.DecodeInt32(bytes.NewBuffer(body[pc:]))
				if err != nil {
					return nil, fmt.Errorf("read immediate: %w", err)
				}
				pc += num - 1
			case OptCodeI64Const:
				_, num, err := leb128.DecodeInt64(bytes.NewBuffer(body[pc:]))
				if err != nil {
					return nil, fmt.Errorf("read immediate: %w", err)
				}
				pc += num - 1
			case OptCodeF32Const:
				pc += 3
			case OptCodeF64Const:
				pc += 7
			}
			continue
		} else if (0x3f <= rawOc && rawOc <= 0x40) || // memory grow,size
			(0x20 <= rawOc && rawOc <= 0x24) || // variable instructions
			(0x0c <= rawOc && rawOc <= 0x0d) || // br,br_if instructions
			(0x10 <= rawOc && rawOc <= 0x11) { // call,call_indirect
			pc++
			_, num, err := leb128.DecodeUint32(bytes.NewBuffer(body[pc:]))
			if err != nil {
				return nil, fmt.Errorf("read immediate: %w", err)
			}
			pc += num - 1
			if rawOc == 0x11 { // if call_indirect
				pc++
			}
			continue
		} else if rawOc == 0x0e { // br_table
			pc++
			r := bytes.NewBuffer(body[pc:])
			nl, num, err := leb128.DecodeUint32(r)
			if err != nil {
				return nil, fmt.Errorf("read immediate: %w", err)
			}

			for i := uint32(0); i < nl; i++ {
				_, n, err := leb128.DecodeUint32(r)
				if err != nil {
					return nil, fmt.Errorf("read immediate: %w", err)
				}
				num += n
			}

			_, n, err := leb128.DecodeUint32(r)
			if err != nil {
				return nil, fmt.Errorf("read immediate: %w", err)
			}
			pc += n + num - 1
			continue
		}

		switch OptCode(rawOc) {
		case OptCodeBlock, OptCodeIf, OptCodeLoop:
			bt, num, err := readBlockType(module, bytes.NewBuffer(body[pc+1:]))
			if err != nil {
				return nil, fmt.Errorf("read block: %w", err)
			}
			stack = append(stack, &NativeFunctionBlock{
				StartAt:        pc,
				BlockType:      bt,
				BlockTypeBytes: num,
			})
			pc += num
		case OptCodeElse:
			stack[len(stack)-1].ElseAt = pc
		case OptCodeEnd:
			bl := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			bl.EndAt = pc
			ret[bl.StartAt] = bl
			if bl.ElseAt <= bl.StartAt {
				// To handle if block without else properly,
				// we set ElseAt to EndAt so we can just skip else.
				bl.ElseAt = bl.EndAt - 1
			}
		}
	}

	if len(stack) > 0 {
		return nil, fmt.Errorf("ill-nested block exists")
	}

	return ret, nil
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
		if raw < 0 || (raw >= int64(len(module.SecTypes))) {
			return nil, 0, fmt.Errorf("invalid block type: %d", raw)
		}
		ret = module.SecTypes[raw]
	}
	return ret, num, nil
}

func (s *Store) AddHostFunction(moduleName, funcName string, fn func(*VirtualMachine) reflect.Value) error {
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
		in := make([]ValueType, p.NumIn())
		for i := range in {
			in[i], err = getTypeOf(p.In(i).Kind())
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

	sig, err := getSignature(fn(&VirtualMachine{}).Type())
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	m.Exports[funcName] = &ExportInstance{
		Kind: ExportKindFunction,
		Addr: len(s.Functions),
	}
	s.Functions = append(s.Functions, &HostFunction{
		Name:             fmt.Sprintf("%s.%s", moduleName, funcName),
		ClosureGenerator: fn,
		Signature:        sig,
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
		Memory: make([]byte, min),
		Min:    min,
		Max:    max,
	})
	return nil
}
