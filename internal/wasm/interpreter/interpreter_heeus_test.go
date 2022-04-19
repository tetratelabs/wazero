package interpreter

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/wasm"
	"github.com/heeus/hwazero/internal/wazeroir"
)

func TestInterpreter_Call_WithContextTimeout(t *testing.T) {
	translateToIROperationKind := func(op wasm.Opcode) (kind wazeroir.OperationKind) {
		switch op {
		case wasm.OpcodeI32Extend8S:
			kind = wazeroir.OperationKindSignExtend32From8
		case wasm.OpcodeI32Extend16S:
			kind = wazeroir.OperationKindSignExtend32From16
		case wasm.OpcodeI64Extend8S:
			kind = wazeroir.OperationKindSignExtend64From8
		case wasm.OpcodeI64Extend16S:
			kind = wazeroir.OperationKindSignExtend64From16
		case wasm.OpcodeI64Extend32S:
			kind = wazeroir.OperationKindSignExtend64From32
		}
		return
	}
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int32
			expected int32
			opcode   wasm.Opcode
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
			{in: 0, expected: 0, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x7f, expected: 127, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x80, expected: -128, opcode: wasm.OpcodeI32Extend8S},
			{in: 0xff, expected: -1, opcode: wasm.OpcodeI32Extend8S},
			{in: 0x012345_00, expected: 0, opcode: wasm.OpcodeI32Extend8S},
			{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, opcode: wasm.OpcodeI32Extend8S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI32Extend8S},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
			{in: 0, expected: 0, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x7fff, expected: 32767, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x8000, expected: -32768, opcode: wasm.OpcodeI32Extend16S},
			{in: 0xffff, expected: -1, opcode: wasm.OpcodeI32Extend16S},
			{in: 0x0123_0000, expected: 0, opcode: wasm.OpcodeI32Extend16S},
			{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, opcode: wasm.OpcodeI32Extend16S},
			{in: -1, expected: -1, opcode: wasm.OpcodeI32Extend16S},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%s(i32.const(0x%x))", wasm.InstructionName(tc.opcode), tc.in), func(t *testing.T) {
				ce := &callEngine{}
				f := &compiledFunction{
					moduleEngine: &moduleEngine{},
					source:       &wasm.FunctionInstance{Module: &wasm.ModuleInstance{}},
					body: []*interpreterOp{
						{kind: wazeroir.OperationKindConstI32, us: []uint64{uint64(uint32(tc.in))}},
						{kind: translateToIROperationKind(tc.opcode)},
						{kind: wazeroir.OperationKindBr, us: []uint64{math.MaxUint64}},
					},
				}
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				var err error
				mdlctx := wasm.NewModule(ctx)
				for i := 0; i < 100000000; i++ {
					err = ce.callNativeFunc(mdlctx, f)
					if nil != err {
						break
					}
					require.Equal(t, tc.expected, int32(uint32(ce.pop())))
				}
				errstr := err.Error()
				require.Equal(t, errstr, "context deadline exceeded")
			})
		}
	})
}
