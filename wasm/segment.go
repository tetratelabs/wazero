package wasm

import (
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

// ImportKind indicates which import description is present
// See https://www.w3.org/TR/wasm-core-1/#import-section%E2%91%A0
type ImportKind = byte

const (
	ImportKindFunc   ImportKind = 0x00
	ImportKindTable  ImportKind = 0x01
	ImportKindMemory ImportKind = 0x02
	ImportKindGlobal ImportKind = 0x03
)

// Import is the binary representation of an import indicated by Kind
// See https://www.w3.org/TR/wasm-core-1/#binary-import
type Import struct {
	Kind ImportKind
	// Module is the possibly empty primary namespace of this import
	Module string
	// Module is the possibly empty secondary namespace of this import
	Name string
	// DescFunc is the index in Module.TypeSection when Kind equals ImportKindFunc
	DescFunc uint32
	// DescTable is the inlined TableType when Kind equals ImportKindTable
	DescTable *TableType
	// DescMem is the inlined MemoryType when Kind equals ImportKindMemory
	DescMem *MemoryType
	// DescGlobal is the inlined GlobalType when Kind equals ImportKindGlobal
	DescGlobal *GlobalType
}

func readImport(r io.Reader) (i *Import, err error) {
	i = &Import{}
	if i.Module, err = readNameValue(r); err != nil {
		return nil, fmt.Errorf("error decoding import module: %w", err)
	}

	if i.Name, err = readNameValue(r); err != nil {
		return nil, fmt.Errorf("error decoding import name: %w", err)
	}

	b := make([]byte, 1)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("error decoding import kind: %w", err)
	}

	i.Kind = b[0]
	switch i.Kind {
	case ImportKindFunc:
		if i.DescFunc, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding import func typeindex: %w", err)
		}
	case ImportKindTable:
		if i.DescTable, err = readTableType(r); err != nil {
			return nil, fmt.Errorf("error decoding import table desc: %w", err)
		}
	case ImportKindMemory:
		if i.DescMem, err = readMemoryType(r); err != nil {
			return nil, fmt.Errorf("error decoding import mem desc: %w", err)
		}
	case ImportKindGlobal:
		if i.DescGlobal, err = readGlobalType(r); err != nil {
			return nil, fmt.Errorf("error decoding import global desc: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for importdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}

type Global struct {
	Type *GlobalType
	Init *ConstantExpression
}

func readGlobal(r io.Reader) (*Global, error) {
	gt, err := readGlobalType(r)
	if err != nil {
		return nil, fmt.Errorf("read global type: %v", err)
	}

	init, err := readConstantExpression(r)
	if err != nil {
		return nil, fmt.Errorf("get init expression: %v", err)
	}

	return &Global{
		Type: gt,
		Init: init,
	}, nil
}

// ExportKind indicates which index Export.Index points to
// See https://www.w3.org/TR/wasm-core-1/#export-section%E2%91%A0
type ExportKind = byte

const (
	ExportKindFunc   ExportKind = 0x00
	ExportKindTable  ExportKind = 0x01
	ExportKindMemory ExportKind = 0x02
	ExportKindGlobal ExportKind = 0x03
)

// Export is the binary representation of an export indicated by Kind
// See https://www.w3.org/TR/wasm-core-1/#binary-export
type Export struct {
	Kind ExportKind
	// Name is what the host refers to this definition as.
	Name string
	// Index is the index of the definition to export, the index namespace is by Kind
	// Ex. If ExportKindFunc, this is an index to ModuleInstance.Functions
	Index uint32
}

func readExport(r io.Reader) (i *Export, err error) {
	i = &Export{}

	if i.Name, err = readNameValue(r); err != nil {
		return nil, fmt.Errorf("error decoding export name: %w", err)
	}

	b := make([]byte, 1)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("error decoding export kind: %w", err)
	}

	i.Kind = b[0]
	switch i.Kind {
	case ExportKindFunc, ExportKindTable, ExportKindMemory, ExportKindGlobal:
		if i.Index, _, err = leb128.DecodeUint32(r); err != nil {
			return nil, fmt.Errorf("error decoding export index: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: invalid byte for exportdesc: %#x", ErrInvalidByte, b[0])
	}
	return
}

type ElementSegment struct {
	TableIndex uint32
	OffsetExpr *ConstantExpression
	Init       []uint32
}

func readElementSegment(r io.Reader) (*ElementSegment, error) {
	ti, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get table index: %w", err)
	}

	expr, err := readConstantExpression(r)
	if err != nil {
		return nil, fmt.Errorf("read expr for offset: %w", err)
	}

	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get size of vector: %w", err)
	}

	init := make([]uint32, vs)
	for i := range init {
		fIDx, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read function index: %w", err)
		}
		init[i] = fIDx
	}

	return &ElementSegment{
		TableIndex: ti,
		OffsetExpr: expr,
		Init:       init,
	}, nil
}

type Code struct {
	NumLocals  uint32
	LocalTypes []ValueType
	Body       []byte
}

func readCode(r io.Reader) (*Code, error) {
	ss, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size of code segment: %w", err)
	}

	r = io.LimitReader(r, int64(ss))

	// parse locals
	ls, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size locals: %v", err)
	}

	var nums []uint64
	var types []ValueType
	var sum uint64
	b := make([]byte, 1)
	for i := uint32(0); i < ls; i++ {
		n, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read n of locals: %v", err)
		}
		sum += uint64(n)
		nums = append(nums, uint64(n))

		_, err = io.ReadFull(r, b)
		if err != nil {
			return nil, fmt.Errorf("read type of local: %v", err)
		}
		switch vt := ValueType(b[0]); vt {
		case ValueTypeI32, ValueTypeF32, ValueTypeI64, ValueTypeF64:
			types = append(types, vt)
		default:
			return nil, fmt.Errorf("invalid local type: 0x%x", vt)
		}
	}

	if sum > math.MaxUint32 {
		return nil, fmt.Errorf("too many locals: %d", sum)
	}

	var localTypes []ValueType
	for i, num := range nums {
		t := types[i]
		for j := uint64(0); j < num; j++ {
			localTypes = append(localTypes, t)
		}
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if body[len(body)-1] != byte(OpcodeEnd) {
		return nil, fmt.Errorf("expr not end with OpcodeEnd")
	}

	return &Code{
		Body:       body,
		NumLocals:  uint32(sum),
		LocalTypes: localTypes,
	}, nil
}

type DataSegment struct {
	MemoryIndex      uint32 // supposed to be zero
	OffsetExpression *ConstantExpression
	Init             []byte
}

func readDataSegment(r io.Reader) (*DataSegment, error) {
	d, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("read memory index: %v", err)
	}

	if d != 0 {
		return nil, fmt.Errorf("invalid memory index: %d", d)
	}

	expr, err := readConstantExpression(r)
	if err != nil {
		return nil, fmt.Errorf("read offset expression: %v", err)
	}

	vs, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("get the size of vector: %v", err)
	}

	b := make([]byte, vs)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read bytes for init: %v", err)
	}

	return &DataSegment{
		OffsetExpression: expr,
		Init:             b,
	}, nil
}
