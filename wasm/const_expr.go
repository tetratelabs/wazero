package wasm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero/wasm/leb128"
)

type ConstantExpression struct {
	Opcode Opcode
	Data   []byte
}

func readConstantExpression(r io.Reader) (*ConstantExpression, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read opcode: %v", err)
	}
	buf := new(bytes.Buffer)
	teeR := io.TeeReader(r, buf)

	opcode := b[0]
	switch opcode {
	case OpcodeI32Const:
		_, _, err = leb128.DecodeInt32(teeR)
	case OpcodeI64Const:
		_, _, err = leb128.DecodeInt64(teeR)
	case OpcodeF32Const:
		_, err = readFloat32(teeR)
	case OpcodeF64Const:
		_, err = readFloat64(teeR)
	case OpcodeGlobalGet:
		_, _, err = leb128.DecodeUint32(teeR)
	default:
		return nil, fmt.Errorf("%v for const expression opt code: %#x", ErrInvalidByte, b[0])
	}

	if err != nil {
		return nil, fmt.Errorf("read value: %v", err)
	}

	if _, err := io.ReadFull(r, b); err != nil {
		return nil, fmt.Errorf("look for end opcode: %v", err)
	}

	if b[0] != byte(OpcodeEnd) {
		return nil, fmt.Errorf("constant expression has been not terminated")
	}

	return &ConstantExpression{
		Opcode: opcode,
		Data:   buf.Bytes(),
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
