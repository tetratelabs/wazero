package binaryencoding

import (
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func encodeDataSegment(d *wasm.DataSegment) (ret []byte) {
	// Currently multiple memories are not supported.
	ret = append(ret, leb128.EncodeInt32(0)...)
	ret = append(ret, encodeConstantExpression(d.OffsetExpression)...)
	ret = append(ret, leb128.EncodeUint32(uint32(len(d.Init)))...)
	ret = append(ret, d.Init...)
	return
}
