package binary

import (
	"bytes"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/ieee754"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

func decodeConstantExpression(r io.Reader) (*wasm.ConstantExpression, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return nil, fmt.Errorf("read opcode: %v", err)
	}
	buf := new(bytes.Buffer)
	teeR := io.TeeReader(r, buf)

	opcode := b[0]
	switch opcode {
	case wasm.OpcodeI32Const:
		_, _, err = leb128.DecodeInt32(teeR)
	case wasm.OpcodeI64Const:
		_, _, err = leb128.DecodeInt64(teeR)
	case wasm.OpcodeF32Const:
		_, err = ieee754.DecodeFloat32(teeR)
	case wasm.OpcodeF64Const:
		_, err = ieee754.DecodeFloat64(teeR)
	case wasm.OpcodeGlobalGet:
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

	if b[0] != byte(wasm.OpcodeEnd) {
		return nil, fmt.Errorf("constant expression has been not terminated")
	}

	return &wasm.ConstantExpression{
		Opcode: opcode,
		Data:   buf.Bytes(),
	}, nil
}
