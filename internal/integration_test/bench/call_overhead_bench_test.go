package bench

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// BenchmarkCallEntryCheckOverhead isolates what `WithCloseOnContextDone(true)` costs per
// *call*, as opposed to per loop iteration (which BenchmarkContextDoneOverhead covers).
// Both engines emit the function-entry check, so both are measured.
//
//   - "tree" is a loop-free call tree, so every check it pays is a function-entry check:
//     f_i calls f_{i+1} twice down to a nop leaf, for 2^depth-1 invocations and zero loop
//     back-edges.
//   - "call_loop" is a loop whose body does nothing but call a nop leaf, so each iteration
//     pays one loop-header check plus one function-entry check.
func BenchmarkCallEntryCheckOverhead(b *testing.B) {
	const treeDepth = 20 // 2^20-1 == ~1.05M invocations per call.
	const loopIters = 1_000_000

	tree := buildCallTreeModule(treeDepth)
	callLoop := buildCallLoopModule()

	for _, engine := range []struct {
		name   string
		newCfg func() wazero.RuntimeConfig
	}{
		{"compiler", wazero.NewRuntimeConfigCompiler},
		{"interpreter", wazero.NewRuntimeConfigInterpreter},
	} {
		b.Run(engine.name, func(b *testing.B) {
			if engine.name == "compiler" && !platform.CompilerSupported() {
				b.Skip()
			}
			for _, ensure := range []bool{false, true} {
				label := "without_close_on_ctx_done"
				if ensure {
					label = "with_close_on_ctx_done"
				}
				b.Run(label, func(b *testing.B) {
					ctx := context.Background()
					cfg := engine.newCfg().WithCloseOnContextDone(ensure)
					r := wazero.NewRuntimeWithConfig(ctx, cfg)
					defer r.Close(ctx)

					treeMod, err := r.Instantiate(ctx, tree)
					require.NoError(b, err)
					loopMod, err := r.Instantiate(ctx, callLoop)
					require.NoError(b, err)

					treeFn := treeMod.ExportedFunction("run")
					loopFn := loopMod.ExportedFunction("run")

					b.Run("tree_1M_calls", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							if _, err := treeFn.Call(ctx); err != nil {
								b.Fatal(err)
							}
						}
					})
					b.Run("call_loop_1M", func(b *testing.B) {
						for i := 0; i < b.N; i++ {
							if _, err := loopFn.Call(ctx, loopIters); err != nil {
								b.Fatal(err)
							}
						}
					})
				})
			}
		})
	}
}

// buildCallTreeModule builds a loop-free call tree: f_i calls f_{i+1} twice, down to a nop
// leaf. f_0 is exported as "run" and performs 2^depth-1 total invocations.
func buildCallTreeModule(depth int) []byte {
	funcs := make([]wasm.Index, depth)
	code := make([]wasm.Code, depth)
	for i := 0; i < depth; i++ {
		funcs[i] = 0
		if i == depth-1 {
			code[i] = wasm.Code{Body: []byte{0x01 /* nop */, opEnd}}
			continue
		}
		callee := byte(i + 1) // depth stays below 128, so a single-byte LEB128 index is fine.
		code[i] = wasm.Code{Body: []byte{0x10 /* call */, callee, 0x10, callee, opEnd}}
	}
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{ParamNumInUint64: 0, ResultNumInUint64: 0}},
		FunctionSection: funcs,
		ExportSection:   []wasm.Export{{Name: "run", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection:     code,
	})
}

// buildCallLoopModule builds `(func (param $n i64) (loop (call $leaf) (br_if $L ...)))`,
// so each iteration pays a loop-header check and a function-entry check.
//
//	Locals of "run": 0=$n, 1=$i.
func buildCallLoopModule() []byte {
	const i64 = wasm.ValueTypeI64
	return binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Params: []wasm.ValueType{i64}, ParamNumInUint64: 1},
			{},
		},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Name: "run", Type: wasm.ExternTypeFunc, Index: 0}},
		CodeSection: []wasm.Code{
			{LocalTypes: []wasm.ValueType{i64}, Body: []byte{
				opLoop, bt0x40,
				0x10, 0x01, // call $leaf
				opLocalGet, 1, opI64Const, 0x01, opI64Add, opLocalSet, 1, // i++
				opLocalGet, 1, opLocalGet, 0, opI64LtS, opBrIf, 0, // if i < n: continue
				opEnd,
				opEnd,
			}},
			{Body: []byte{0x01 /* nop */, opEnd}},
		},
	})
}
