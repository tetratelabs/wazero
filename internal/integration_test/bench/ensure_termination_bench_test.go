package bench

// What WithCloseOnContextDone costs a loop-heavy module: the same wasm, on a runtime with
// ensureTermination off and on. With it on, each loop back-edge tests the module's Closed
// word inline and also bumps a counter that forces an exit to Go every Nth back-edge
// (the exit is a GC safepoint: Go cannot asynchronously preempt goroutines running
// generated machine code), so a module whose work IS a loop pays the loads and branches
// on every iteration but rarely exits.
//
//	go test -bench BenchmarkEnsureTermination -benchtime 2s -count 5 ./internal/integration_test/bench/

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// blockTypeEmpty is the block type of a block or loop producing no results.
const blockTypeEmpty = 0x40

// sumLoopWasm is a module whose only export is a counting loop:
//
//	(module
//	  (func (export "sum") (param i32) (result i64) (local i64)
//	    (block
//	      (loop
//	        (br_if 1 (i32.eqz (local.get 0)))
//	        (local.set 1 (i64.add (local.get 1) (i64.extend_i32_u (local.get 0))))
//	        (local.set 0 (i32.sub (local.get 0) (i32.const 1)))
//	        (br 0)))
//	    (local.get 1)))
func sumLoopWasm() []byte {
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{{
			Params:  []wasm.ValueType{wasm.ValueTypeI32},
			Results: []wasm.ValueType{wasm.ValueTypeI64},
		}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Name: "sum", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection: []wasm.Code{{
			LocalTypes: []wasm.ValueType{wasm.ValueTypeI64}, // the accumulator
			Body: []byte{
				wasm.OpcodeBlock, blockTypeEmpty,
				wasm.OpcodeLoop, blockTypeEmpty,
				// Leave the loop once the counter reaches zero.
				wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Eqz, wasm.OpcodeBrIf, 1,
				// acc += u64(n)
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 0, wasm.OpcodeI64ExtendI32U,
				wasm.OpcodeI64Add, wasm.OpcodeLocalSet, 1,
				// n -= 1
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI32Const, 1, wasm.OpcodeI32Sub, wasm.OpcodeLocalSet, 0,
				wasm.OpcodeBr, 0,
				wasm.OpcodeEnd, wasm.OpcodeEnd, // end loop, end block
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeEnd, // end func
			},
		}},
	})
}

func BenchmarkEnsureTermination(b *testing.B) {
	const iterations = 1 << 16
	wasmBytes := sumLoopWasm()
	for _, tc := range []struct {
		name              string
		ensureTermination bool
	}{{"off", false}, {"on", true}} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx,
				wazero.NewRuntimeConfigCompiler().WithCloseOnContextDone(tc.ensureTermination))
			defer r.Close(ctx)
			m, err := r.Instantiate(ctx, wasmBytes)
			if err != nil {
				b.Fatal(err)
			}
			sum := m.ExportedFunction("sum")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sum.Call(ctx, iterations); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
