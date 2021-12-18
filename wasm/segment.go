package wasm

import (
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

type ImportKind = byte

const (
	ImportKindFunction ImportKind = 0x00
	ImportKindTable    ImportKind = 0x01
	ImportKindMemory   ImportKind = 0x02
	ImportKindGlobal   ImportKind = 0x03
)

type ImportDesc struct {
	Kind byte

	TypeIndexPtr  *uint32
	TableTypePtr  *TableType
	MemTypePtr    *MemoryType
	GlobalTypePtr *GlobalType
}

func readImportDesc(r io.Reader) (*ImportDesc, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read value kind: %v", err)
	}

	switch b[0] {
	case ImportKindFunction:
		tID, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read typeindex: %v", err)
		}
		return &ImportDesc{
			Kind:         ImportKindFunction,
			TypeIndexPtr: &tID,
		}, nil
	case ImportKindTable:
		tt, err := readTableType(r)
		if err != nil {
			return nil, fmt.Errorf("read table type: %v", err)
		}
		return &ImportDesc{
			Kind:         ImportKindTable,
			TableTypePtr: tt,
		}, nil
	case ImportKindMemory:
		mt, err := readMemoryType(r)
		if err != nil {
			return nil, fmt.Errorf("read table type: %v", err)
		}
		return &ImportDesc{
			Kind:       ImportKindMemory,
			MemTypePtr: mt,
		}, nil
	case ImportKindGlobal:
		gt, err := readGlobalType(r)
		if err != nil {
			return nil, fmt.Errorf("read global type: %v", err)
		}
		return &ImportDesc{
			Kind:          ImportKindGlobal,
			GlobalTypePtr: gt,
		}, nil
	default:
		return nil, fmt.Errorf("%w: invalid byte for importdesc: %#x", ErrInvalidByte, b[0])
	}
}

type ImportSegment struct {
	Module, Name string
	Desc         *ImportDesc
}

func readImportSegment(r io.Reader) (*ImportSegment, error) {
	mn, err := readNameValue(r)
	if err != nil {
		return nil, fmt.Errorf("read name of imported module: %v", err)
	}

	n, err := readNameValue(r)
	if err != nil {
		return nil, fmt.Errorf("read name of imported module component: %v", err)
	}

	d, err := readImportDesc(r)
	if err != nil {
		return nil, fmt.Errorf("read import description : %v", err)
	}

	return &ImportSegment{Module: mn, Name: n, Desc: d}, nil
}

type GlobalSegment struct {
	Type *GlobalType
	Init *ConstantExpression
}

func readGlobalSegment(r io.Reader) (*GlobalSegment, error) {
	gt, err := readGlobalType(r)
	if err != nil {
		return nil, fmt.Errorf("read global type: %v", err)
	}

	init, err := readConstantExpression(r)
	if err != nil {
		return nil, fmt.Errorf("get init expression: %v", err)
	}

	return &GlobalSegment{
		Type: gt,
		Init: init,
	}, nil
}

type ExportDesc struct {
	Kind  byte
	Index uint32
}

type ExportKind = byte

const (
	ExportKindFunction ExportKind = 0x00
	ExportKindTable    ExportKind = 0x01
	ExportKindMemory   ExportKind = 0x02
	ExportKindGlobal   ExportKind = 0x03
)

func readExportDesc(r io.Reader) (*ExportDesc, error) {
	b := make([]byte, 1)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("read value kind: %w", err)
	}

	kind := b[0]
	if kind >= 0x04 {
		return nil, fmt.Errorf("%w: invalid byte for exportdesc: %#x", ErrInvalidByte, kind)
	}

	id, _, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("read funcidx: %w", err)
	}

	return &ExportDesc{
		Kind:  kind,
		Index: id,
	}, nil

}

type ExportSegment struct {
	Name string
	Desc *ExportDesc
}

func readExportSegment(r io.Reader) (*ExportSegment, error) {
	name, err := readNameValue(r)
	if err != nil {
		return nil, fmt.Errorf("read name of export module: %w", err)
	}

	d, err := readExportDesc(r)
	if err != nil {
		return nil, fmt.Errorf("read export description: %w", err)
	}

	return &ExportSegment{Name: name, Desc: d}, nil
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

type CodeSegment struct {
	NumLocals  uint32
	LocalTypes []ValueType
	Body       []byte
}

func readCodeSegment(r io.Reader) (*CodeSegment, error) {
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

	return &CodeSegment{
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
