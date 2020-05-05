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
	optCode OptCode
	data    []byte
}

func (m *Module) executeConstExpression(expr *ConstantExpression) (v interface{}, err error) {
	r := bytes.NewBuffer(expr.data)
	switch expr.optCode {
	case OptCodeI32Const:
		v, _, err = leb128.DecodeInt32(r)
		if err != nil {
			return nil, fmt.Errorf("read int32: %w", err)
		}
	case OptCodeI64Const:
		v, _, err = leb128.DecodeInt64(r)
		if err != nil {
			return nil, fmt.Errorf("read int64: %w", err)
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
		if uint32(len(m.IndexSpace.Globals)) <= id {
			return nil, fmt.Errorf("global index out of range")
		}
		v = m.IndexSpace.Globals[id].Val
	default:
		return nil, fmt.Errorf("invalid opt code: %#x", expr.optCode)
	}
	return v, nil
}

func readConstantExpression(r io.Reader) (*ConstantExpression, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read optcode: %w", err)
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
		return nil, fmt.Errorf("%w for opt code: %#x", ErrInvalidByte, b[0])
	}

	if err != nil {
		return nil, fmt.Errorf("read value: %w", err)
	}

	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("look for end optcode: %w", err)
	}

	if b[0] != byte(OptCodeEnd) {
		return nil, fmt.Errorf("constant expression has not terminated")
	}

	return &ConstantExpression{
		optCode: optCode,
		data:    buf.Bytes(),
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
