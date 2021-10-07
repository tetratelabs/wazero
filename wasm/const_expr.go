package wasm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/mathetake/gasm/wasm/leb128"
)

type ConstantExpression struct {
	OptCode OptCode
	Data    []byte
}

func (s *Store) executeConstExpression(target *ModuleInstance, expr *ConstantExpression) (v interface{}, err error) {
	r := bytes.NewBuffer(expr.Data)
	switch expr.OptCode {
	case OptCodeI32Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, fmt.Errorf("read uint32: %w", err)
		}
	case OptCodeI64Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, fmt.Errorf("read uint64: %w", err)
		}
	case OptCodeF32Const:
		v, err = readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("read f34: %w", err)
		}
	case OptCodeF64Const:
		v, err = readFloat64(r)
		if err != nil {
			return nil, fmt.Errorf("read f64: %w", err)
		}
	case OptCodeGlobalGet:
		id, _, err := leb128.DecodeUint32(r)
		if err != nil {
			return nil, fmt.Errorf("read index of global: %w", err)
		}
		if uint32(len(target.GlobalsAddrs)) <= id {
			return nil, fmt.Errorf("global index out of range")
		}
		g := s.Globals[target.GlobalsAddrs[id]]
		switch g.Type.ValType {
		case ValueTypeI32:
			v = int32(g.Val)
		case ValueTypeI64:
			v = int64(g.Val)
		case ValueTypeF32:
			v = math.Float32frombits(uint32(g.Val))
		case ValueTypeF64:
			v = math.Float64frombits(uint64(g.Val))
		}
	default:
		return nil, fmt.Errorf("invalid opt code: %#x", expr.OptCode)
	}
	return v, nil
}

func readConstantExpression(r io.Reader) (*ConstantExpression, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read optcode: %v", err)
	}
	buf := new(bytes.Buffer)
	teeR := io.TeeReader(r, buf)

	optCode := OptCode(b[0])
	switch optCode {
	case OptCodeI32Const:
		_, _, err = leb128.DecodeInt32(teeR)
	case OptCodeI64Const:
		_, _, err = leb128.DecodeInt64(teeR)
	case OptCodeF32Const:
		_, err = readFloat32(teeR)
	case OptCodeF64Const:
		_, err = readFloat64(teeR)
	case OptCodeGlobalGet:
		_, _, err = leb128.DecodeUint32(teeR)
	default:
		return nil, fmt.Errorf("%v for const expression opt code: %#x", ErrInvalidByte, b[0])
	}

	if err != nil {
		return nil, fmt.Errorf("read value: %v", err)
	}

	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("look for end optcode: %v", err)
	}

	if b[0] != byte(OptCodeEnd) {
		return nil, fmt.Errorf("constant expression has been not terminated")
	}

	return &ConstantExpression{
		OptCode: optCode,
		Data:    buf.Bytes(),
	}, nil
}

// IEEE 754
func readFloat32(r io.Reader) (float32, error) {
	buf := make([]byte, 4)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	raw := binary.LittleEndian.Uint32(buf)
	return math.Float32frombits(raw), nil
}

// IEEE 754
func readFloat64(r io.Reader) (float64, error) {
	buf := make([]byte, 8)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return 0, err
	}
	raw := binary.LittleEndian.Uint64(buf)
	return math.Float64frombits(raw), nil
}
