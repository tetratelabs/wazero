package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/wasm"
)

func encodeConstantExpression(expr wasm.ConstantExpression) (ret []byte) {
	ret = append(ret, expr.Opcode)
	ret = append(ret, expr.Data...)
	ret = append(ret, wasm.OpcodeEnd)
	return
}
