package binaryencoding

import (
	"bytes"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func ensureElementKindFuncRef(r *bytes.Reader) error {
	elemKind, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("read element prefix: %w", err)
	}
	if elemKind != 0x0 { // ElemKind is fixed to 0x0 now: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/binary/modules.html#element-section
		return fmt.Errorf("element kind must be zero but was 0x%x", elemKind)
	}
	return nil
}

// encodeCode returns the wasm.ElementSegment encoded in WebAssembly 1.0 (20191205) Binary Format.
//
// https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#element-section%E2%91%A0
func encodeElement(e *wasm.ElementSegment) (ret []byte) {
	if e.Mode == wasm.ElementModeActive {
		ret = append(ret, leb128.EncodeInt32(int32(e.TableIndex))...)
		ret = append(ret, encodeConstantExpression(e.OffsetExpr)...)
		ret = append(ret, leb128.EncodeUint32(uint32(len(e.Init)))...)
		for _, idx := range e.Init {
			ret = append(ret, leb128.EncodeInt32(int32(idx))...)
		}
	} else {
		panic("TODO: support encoding for non-active elements in bulk-memory-operations proposal")
	}
	return
}
