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

// BenchmarkContextDoneOverhead isolates the cost paid by `WithCloseOnContextDone(true)`.
//
// Every wasm `loop` opcode header emits a module closed check when ensureTermination is on;
// every Wasm call also spawns a watchdog goroutine via CloseModuleOnCanceledOrTimeout.
// We vary iterations-per-call so the two cost regimes can be separated:
//
//   - "short" (1k iters/call): per-call costs (goroutine spawn + close-chan wakeup) dominate.
//   - "long"  (10M iters/call): per-iteration trampoline cost dominates; per-call cost is amortized.
//
// "tight" = pure integer loop (back-edge is the only work).
// "mem"   = same loop but reads i64 from linear memory each iteration.
func BenchmarkContextDoneOverhead(b *testing.B) {
	if !platform.CompilerSupported() {
		b.Skip()
	}

	bin := buildContextDoneModule(b)

	itersByName := []struct {
		name  string
		iters uint64
	}{
		{"short_1k", 1_000},
		{"long_10M", 10_000_000},
	}

	for _, ensure := range []bool{false, true} {
		ensure := ensure
		label := "without"
		if ensure {
			label = "with"
		}
		b.Run(label+"_close_on_ctx_done", func(b *testing.B) {
			ctx := context.Background()
			cfg := wazero.NewRuntimeConfigCompiler().WithCloseOnContextDone(ensure)
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)

			mod, err := r.Instantiate(ctx, bin)
			require.NoError(b, err)

			tightLoop := mod.ExportedFunction("tight_loop")
			memLoop := mod.ExportedFunction("mem_loop")

			for _, sz := range itersByName {
				sz := sz
				b.Run("tight/"+sz.name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						if _, err := tightLoop.Call(ctx, sz.iters); err != nil {
							b.Fatal(err)
						}
					}
				})
				b.Run("mem/"+sz.name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						if _, err := memLoop.Call(ctx, sz.iters); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// buildContextDoneModule produces a wasm module with two exported functions:
//
//	(func (export "tight_loop") (param i64) (result i64) ...)
//	(func (export "mem_loop")   (param i64) (result i64) ...)
//
// Both run a tight loop for `n` iterations. mem_loop additionally reads an i64 from
// linear memory each iteration. Hand-encoded to avoid pulling in a wat parser.
func buildContextDoneModule(b *testing.B) []byte {
	b.Helper()
	const i64 = wasm.ValueTypeI64

	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection: []wasm.FunctionType{{
			Params:           []wasm.ValueType{i64},
			Results:          []wasm.ValueType{i64},
			ParamNumInUint64: 1, ResultNumInUint64: 1,
		}},
		FunctionSection: []wasm.Index{0, 0},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 1, IsMaxEncoded: true},
		ExportSection: []wasm.Export{
			{Name: "tight_loop", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "mem_loop", Type: wasm.ExternTypeFunc, Index: 1},
		},
		CodeSection: []wasm.Code{
			{LocalTypes: []wasm.ValueType{i64, i64}, Body: tightLoopBody()},
			{LocalTypes: []wasm.ValueType{i64, i64}, Body: memLoopBody()},
		},
	})
	return bin
}

// Wasm opcodes used below.
const (
	opLocalGet   = 0x20
	opLocalSet   = 0x21
	opI64Const   = 0x42
	opI64Add     = 0x7c
	opI64And     = 0x83
	opI64LtS     = 0x53
	opI64Load    = 0x29
	opI32WrapI64 = 0xa7
	opLoop       = 0x03
	opEnd        = 0x0b
	opBrIf       = 0x0d
	bt0x40       = 0x40 // empty block type
)

// tightLoopBody:
//
//	(func (param $n i64) (result i64) (local $i i64) (local $sum i64)
//	  (loop $L
//	    (local.set $sum (i64.add (local.get $sum) (local.get $i)))
//	    (local.set $i   (i64.add (local.get $i) (i64.const 1)))
//	    (br_if $L (i64.lt_s (local.get $i) (local.get $n))))
//	  (local.get $sum))
//
// Locals: 0=$n, 1=$i, 2=$sum.
func tightLoopBody() []byte {
	return []byte{
		opLoop, bt0x40,
		opLocalGet, 2, opLocalGet, 1, opI64Add, opLocalSet, 2, // sum += i
		opLocalGet, 1, opI64Const, 0x01, opI64Add, opLocalSet, 1, // i++
		opLocalGet, 1, opLocalGet, 0, opI64LtS, opBrIf, 0, // if i < n: continue
		opEnd,
		opLocalGet, 2,
		opEnd,
	}
}

// memLoopBody: same as tightLoopBody, but `sum += mem[(i & 0xfff8)]` instead of `sum += i`.
// The mask keeps the address inside the single-page memory and 8-byte aligned.
func memLoopBody() []byte {
	// i64.const 0xfff8 in signed leb128: 0xf8 0xff 0x03
	return []byte{
		opLoop, bt0x40,
		opLocalGet, 2, // sum
		opLocalGet, 1, // i
		opI64Const, 0xf8, 0xff, 0x03, // 0xfff8
		opI64And,              // i & mask
		opI32WrapI64,          // -> i32 address
		opI64Load, 0x03, 0x00, // align=3 offset=0
		opI64Add, opLocalSet, 2, // sum += mem[i & mask]
		opLocalGet, 1, opI64Const, 0x01, opI64Add, opLocalSet, 1, // i++
		opLocalGet, 1, opLocalGet, 0, opI64LtS, opBrIf, 0,
		opEnd,
		opLocalGet, 2,
		opEnd,
	}
}
