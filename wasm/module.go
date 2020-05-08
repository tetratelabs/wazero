package wasm

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/mathetake/gasm/wasm/leb128"
)

var (
	magic   = []byte{0x00, 0x61, 0x73, 0x6D}
	version = []byte{0x01, 0x00, 0x00, 0x00}

	ErrInvalidMagicNumber = errors.New("invalid magic number")
	ErrInvalidVersion     = errors.New("invalid version header")
)

type (
	Module struct {
		SecTypes     []*FunctionType
		SecImports   []*ImportSegment
		SecFunctions []uint32
		SecTables    []*TableType
		SecMemory    []*MemoryType
		SecGlobals   []*GlobalSegment
		SecExports   map[string]*ExportSegment
		SecStart     []uint32
		SecElements  []*ElementSegment
		SecCodes     []*CodeSegment
		SecData      []*DataSegment

		IndexSpace *ModuleIndexSpace
	}

	ModuleIndexSpace struct {
		Function []VirtualMachineFunction
		Globals  []*Global
		Table    [][]*uint32
		Memory   [][]byte
	}

	// initialized global
	Global struct {
		Type *GlobalType
		Val  interface{}
	}
)

// DecodeModule decodes a `raw` module from io.Reader whose index spaces are yet to be initialized
func DecodeModule(r io.Reader) (*Module, error) {
	// magic number
	buf := make([]byte, 4)
	if n, err := io.ReadFull(r, buf); err != nil || n != 4 {
		return nil, ErrInvalidMagicNumber
	}
	for i := 0; i < 4; i++ {
		if buf[i] != magic[i] {
			return nil, ErrInvalidMagicNumber
		}
	}

	// version
	if n, err := io.ReadFull(r, buf); err != nil || n != 4 {
		panic(err)
	}
	for i := 0; i < 4; i++ {
		if buf[i] != version[i] {
			return nil, ErrInvalidVersion
		}
	}

	ret := &Module{}
	if err := ret.readSections(r); err != nil {
		return nil, fmt.Errorf("readSections failed: %w", err)
	}

	return ret, nil
}

// buildIndexSpaces build index spaces of the module with the given external modules
func (m *Module) buildIndexSpaces(externModules map[string]*Module) error {
	m.IndexSpace = new(ModuleIndexSpace)

	// resolve imports
	if err := m.resolveImports(externModules); err != nil {
		return fmt.Errorf("resolve imports: %w", err)
	}

	// fill in the gap between the definition and imported ones in index spaces
	// note: MVP restricts the size of memory index spaces to 1
	if diff := len(m.SecTables) - len(m.IndexSpace.Table); diff > 0 {
		for i := 0; i < diff; i++ {
			m.IndexSpace.Table = append(m.IndexSpace.Table, []*uint32{})
		}
	}

	// fill in the gap between the definition and imported ones in index spaces
	// note: MVP restricts the size of memory index spaces to 1
	if diff := len(m.SecMemory) - len(m.IndexSpace.Memory); diff > 0 {
		for i := 0; i < diff; i++ {
			m.IndexSpace.Memory = append(m.IndexSpace.Memory, []byte{})
		}
	}

	if err := m.buildGlobalIndexSpace(); err != nil {
		return fmt.Errorf("build global index space: %w", err)
	}
	if err := m.buildFunctionIndexSpace(); err != nil {
		return fmt.Errorf("build function index space: %w", err)
	}
	if err := m.buildTableIndexSpace(); err != nil {
		return fmt.Errorf("build table index space: %w", err)
	}
	if err := m.buildMemoryIndexSpace(); err != nil {
		return fmt.Errorf("build memory index space: %w", err)
	}
	return nil
}

func (m *Module) resolveImports(externModules map[string]*Module) error {
	for _, is := range m.SecImports {
		em, ok := externModules[is.Module]
		if !ok {
			return fmt.Errorf("failed to resolve import of module name %s", is.Module)
		}

		es, ok := em.SecExports[is.Name]
		if !ok {
			return fmt.Errorf("%s not exported in module %s", is.Name, is.Module)
		}

		if is.Desc.Kind != es.Desc.Kind {
			return fmt.Errorf("type mismatch on export: got %#x but want %#x", es.Desc.Kind, is.Desc.Kind)
		}
		switch is.Desc.Kind {
		case 0x00: // function
			if err := m.applyFunctionImport(is, em, es); err != nil {
				return fmt.Errorf("applyFunctionImport failed: %w", err)
			}
		case 0x01: // table
			if err := m.applyTableImport(em, es); err != nil {
				return fmt.Errorf("applyTableImport failed: %w", err)
			}
		case 0x02: // mem
			if err := m.applyMemoryImport(em, es); err != nil {
				return fmt.Errorf("applyMemoryImport: %w", err)
			}
		case 0x03: // global
			if err := m.applyGlobalImport(em, es); err != nil {
				return fmt.Errorf("applyGlobalImport: %w", err)
			}
		default:
			return fmt.Errorf("invalid kind of import: %#x", is.Desc.Kind)
		}
	}
	return nil
}

func (m *Module) applyFunctionImport(is *ImportSegment, em *Module, es *ExportSegment) error {
	if es.Desc.Index >= uint32(len(em.IndexSpace.Function)) {
		return fmt.Errorf("exported index out of range")
	}

	if is.Desc.TypeIndexPtr == nil {
		return fmt.Errorf("is.Desc.TypeIndexPtr is nill")
	}

	iSig := m.SecTypes[*is.Desc.TypeIndexPtr]
	f := em.IndexSpace.Function[es.Desc.Index]
	if !hasSameSignature(iSig.ReturnTypes, f.FunctionType().ReturnTypes) {
		return fmt.Errorf("return signature mimatch: %#x != %#x", iSig.ReturnTypes, f.FunctionType().ReturnTypes)
	} else if !hasSameSignature(iSig.InputTypes, f.FunctionType().InputTypes) {
		return fmt.Errorf("input signature mimatch: %#x != %#x", iSig.InputTypes, f.FunctionType().InputTypes)
	}
	m.IndexSpace.Function = append(m.IndexSpace.Function, f)
	return nil
}

func (m *Module) applyTableImport(em *Module, es *ExportSegment) error {
	if es.Desc.Index >= uint32(len(em.IndexSpace.Table)) {
		return fmt.Errorf("exported index out of range")
	}

	// note: MVP restricts the size of table index spaces to 1
	m.IndexSpace.Table = append(m.IndexSpace.Table, em.IndexSpace.Table[es.Desc.Index])
	return nil
}

func (m *Module) applyMemoryImport(em *Module, es *ExportSegment) error {
	if es.Desc.Index >= uint32(len(em.IndexSpace.Memory)) {
		return fmt.Errorf("exported index out of range")
	}

	// note: MVP restricts the size of memory index spaces to 1
	m.IndexSpace.Memory = append(m.IndexSpace.Memory, em.IndexSpace.Memory[es.Desc.Index])
	return nil
}

func (m *Module) applyGlobalImport(em *Module, es *ExportSegment) error {
	if es.Desc.Index >= uint32(len(em.IndexSpace.Globals)) {
		return fmt.Errorf("exported index out of range")
	}

	gb := em.IndexSpace.Globals[es.Desc.Index]
	if gb.Type.Mutable {
		return fmt.Errorf("cannot import mutable global")
	}

	m.IndexSpace.Globals = append(em.IndexSpace.Globals, gb)
	return nil
}

func (m *Module) buildGlobalIndexSpace() error {
	for _, gs := range m.SecGlobals {
		v, err := m.executeConstExpression(gs.Init)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}
		m.IndexSpace.Globals = append(m.IndexSpace.Globals, &Global{
			Type: gs.Type,
			Val:  v,
		})
	}
	return nil
}

func (m *Module) buildFunctionIndexSpace() error {
	for codeIndex, typeIndex := range m.SecFunctions {
		if typeIndex >= uint32(len(m.SecTypes)) {
			return fmt.Errorf("function type index out of range")
		} else if codeIndex >= len(m.SecCodes) {
			return fmt.Errorf("code index out of range")
		}

		f := &NativeFunction{
			Signature: m.SecTypes[typeIndex],
			Body:      m.SecCodes[codeIndex].Body,
			NumLocal:  m.SecCodes[codeIndex].NumLocals,
		}

		brs, err := m.parseBlocks(f.Body)
		if err != nil {
			return fmt.Errorf("parse blocks: %w", err)
		}
		f.Blocks = brs
		m.IndexSpace.Function = append(m.IndexSpace.Function, f)
	}
	return nil
}

func (m *Module) buildMemoryIndexSpace() error {
	for _, d := range m.SecData {
		// note: MVP restricts the size of memory index spaces to 1
		if d.MemoryIndex >= uint32(len(m.IndexSpace.Memory)) {
			return fmt.Errorf("index out of range of index space")
		} else if d.MemoryIndex >= uint32(len(m.SecMemory)) {
			return fmt.Errorf("index out of range of memory section")
		}

		rawOffset, err := m.executeConstExpression(d.OffsetExpression)
		if err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}

		offset, ok := rawOffset.(int32)
		if !ok {
			return fmt.Errorf("type assertion failed")
		}

		size := int(offset) + len(d.Init)
		if m.SecMemory[d.MemoryIndex].Max != nil && uint32(size) > *(m.SecMemory[d.MemoryIndex].Max)*vmPageSize {
			return fmt.Errorf("memory size out of limit %d * 64Ki", int(*(m.SecMemory[d.MemoryIndex].Max)))
		}

		memory := m.IndexSpace.Memory[d.MemoryIndex]
		if size > len(memory) {
			next := make([]byte, size)
			copy(next, memory)
			copy(next[offset:], d.Init)
			m.IndexSpace.Memory[d.MemoryIndex] = next
		} else {
			copy(memory[offset:], d.Init)
		}
	}
	return nil
}

func (m *Module) buildTableIndexSpace() error {
	for _, elem := range m.SecElements {
		// note: MVP restricts the size of memory index spaces to 1
		if elem.TableIndex >= uint32(len(m.IndexSpace.Table)) {
			return fmt.Errorf("index out of range of index space")
		} else if elem.TableIndex >= uint32(len(m.SecTables)) {
			// this is just in case since we could assume len(SecTables) == len(IndexSpace.Table)
			return fmt.Errorf("index out of range of table section")
		}

		rawOffset, err := m.executeConstExpression(elem.OffsetExpr)
		if err != nil {
			return fmt.Errorf("calculate offset: %w", err)
		}

		offset32, ok := rawOffset.(int32)
		if !ok {
			return fmt.Errorf("type assertion failed")
		}

		offset := int(offset32)
		size := offset + len(elem.Init)
		if m.SecTables[elem.TableIndex].Limit.Max != nil &&
			size > int(*(m.SecTables[elem.TableIndex].Limit.Max)) {
			return fmt.Errorf("table size out of limit of %d", int(*(m.SecTables[elem.TableIndex].Limit.Max)))
		}

		table := m.IndexSpace.Table[elem.TableIndex]
		if size > len(table) {
			next := make([]*uint32, size)
			copy(next, table)
			for i, b := range elem.Init {
				next[i+offset] = &b
			}
			m.IndexSpace.Table[elem.TableIndex] = next
		} else {
			for i, b := range elem.Init {
				table[i+offset] = &b
			}
		}
	}
	return nil
}

type BlockType = FunctionType

func (m *Module) readBlockType(r io.Reader) (*BlockType, uint64, error) {
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
		if raw < 0 || (raw >= int64(len(m.SecTypes))) {
			return nil, 0, fmt.Errorf("invalid block type: %d", raw)
		}
		ret = m.SecTypes[raw]
	}
	return ret, num, nil
}

func (m *Module) parseBlocks(body []byte) (map[uint64]*NativeFunctionBlock, error) {
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
			bt, num, err := m.readBlockType(bytes.NewBuffer(body[pc+1:]))
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
		}
	}

	if len(stack) > 0 {
		return nil, fmt.Errorf("ill-nested block exists")
	}

	return ret, nil
}
