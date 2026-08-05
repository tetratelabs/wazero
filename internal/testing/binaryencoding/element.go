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
		for _, expr := range e.Init {
			if expr.Data[0] == wasm.OpcodeRefFunc {
				u32, n, _ := leb128.DecodeUint32(bytes.NewReader(expr.Data[1:]))
				ind := uint32(u32)
				ret = append(ret, leb128.EncodeUint32(ind)...)
				if expr.Data[1+n] != wasm.OpcodeEnd {
					panic("only single op ref.func is supported for active element encoding")
				}
			} else {
				panic("only ref.func is supported for active element encoding")
			}
		}
	} else {
		panic("TODO: support encoding for non-active elements in bulk-memory-operations proposal")
	}
	return
}
