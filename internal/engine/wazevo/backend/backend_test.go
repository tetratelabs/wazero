package backend_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/frontend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestMain(m *testing.M) {
	if runtime.GOARCH != "arm64" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func newMachine() backend.Machine {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.NewBackend()
	default:
		panic("unsupported architecture")
	}
}

func TestE2E(t *testing.T) {
	const verbose = true

	type testCase struct {
		name                                   string
		m                                      *wasm.Module
		targetIndex                            uint32
		afterLoweringARM64, afterFinalizeARM64 string
		// TODO: amd64.
	}

	for _, tc := range []testCase{
		{
			name: "empty", m: testcases.Empty.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "selects", m: testcases.Selects.Module,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	subs xzr, x4, x5
	csel w0, w2, w3, eq
	subs wzr, w3, wzr
	csel x1, x4, x5, ne
	fcmp d2, d3
	fcsel s8, s0, s1, gt
	fcmp s0, s1
	fcsel d1, d2, d3, ne
	mov v0.8b, v8.8b
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "consts", m: testcases.Constants.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	ldr d5?, #8; b 16; data.f64 64.000000
	mov v1.8b, v5?.8b
	ldr s4?, #8; b 8; data.f32 32.000000
	mov v0.8b, v4?.8b
	orr x3?, xzr, #0x2
	mov x1, x3?
	orr w2?, wzr, #0x1
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	ldr d1, #8; b 16; data.f64 64.000000
	ldr s0, #8; b 8; data.f32 32.000000
	orr x1, xzr, #0x2
	orr w0, wzr, #0x1
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "add sub params return", m: testcases.AddSubParamsReturn.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	add w4?, w2?, w3?
	sub w5?, w4?, w2?
	mov x0, x5?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	add w8, w2, w3
	sub w0, w8, w2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "locals params", m: testcases.LocalsParams.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov v3?.8b, v0.8b
	mov v4?.8b, v1.8b
	add x5?, x2?, x2?
	sub x6?, x5?, x2?
	fadd s7?, s3?, s3?
	fsub s8?, s7?, s3?
	fmul s9?, s8?, s3?
	fdiv s10?, s9?, s3?
	fmax s11?, s10?, s3?
	fmin s12?, s11?, s3?
	fadd d13?, d4?, d4?
	fsub d14?, d13?, d4?
	fmul d15?, d14?, d4?
	fdiv d16?, d15?, d4?
	fmax d17?, d16?, d4?
	fmin d18?, d17?, d4?
	mov v1.8b, v18?.8b
	mov v0.8b, v12?.8b
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	add x8, x2, x2
	sub x0, x8, x2
	fadd s8, s0, s0
	fsub s8, s8, s0
	fmul s8, s8, s0
	fdiv s8, s8, s0
	fmax s8, s8, s0
	fmin s0, s8, s0
	fadd d8, d1, d1
	fsub d8, d8, d1
	fmul d8, d8, d1
	fdiv d8, d8, d1
	fmax d8, d8, d1
	fmin d1, d8, d1
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "local_param_return", m: testcases.LocalParamReturn.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, xzr
	mov x1, x3?
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x1, xzr
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "swap_param_and_return", m: testcases.SwapParamAndReturn.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov x1, x2?
	mov x0, x3?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x1, x2
	mov x0, x3
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "swap_params_and_return", m: testcases.SwapParamsAndReturn.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
L2 (SSA Block: blk1):
	mov x1, x2?
	mov x0, x3?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	mov x1, x2
	mov x0, x3
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "block_br", m: testcases.BlockBr.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
L2 (SSA Block: blk1):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "block_br_if", m: testcases.BlockBrIf.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x3?, xzr
	cbz w3?, (L2)
L3 (SSA Block: blk1):
	ret
L2 (SSA Block: blk2):
	movz x4?, #0x3, lsl 0
	str w4?, [x0?]
	mov x5?, sp
	str x5?, [x0?, #0x38]
	adr x6?, #0x0
	str x6?, [x0?, #0x30]
	exit_sequence x0?
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbz w8, #0x10 L2
L3 (SSA Block: blk1):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk2):
	movz x8, #0x3, lsl 0
	str w8, [x0]
	mov x8, sp
	str x8, [x0, #0x38]
	adr x8, #0x0
	str x8, [x0, #0x30]
	exit_sequence x0
`,
		},
		{
			name: "loop_br", m: testcases.LoopBr.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
L2 (SSA Block: blk1):
	b L2
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	b #0x0 (L2)
`,
		},
		{
			name: "loop_with_param_results", m: testcases.LoopBrWithParamResults.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
L2 (SSA Block: blk1):
	orr w5?, wzr, #0x1
	cbz w5?, (L3)
L4 (SSA Block: blk4):
	b L2
L3 (SSA Block: blk3):
L5 (SSA Block: blk2):
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	orr w8, wzr, #0x1
	cbz w8, #0x8 L3
L4 (SSA Block: blk4):
	b #-0x8 (L2)
L3 (SSA Block: blk3):
L5 (SSA Block: blk2):
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "loop_br_if", m: testcases.LoopBrIf.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
L2 (SSA Block: blk1):
	orr w3?, wzr, #0x1
	cbz w3?, (L3)
L4 (SSA Block: blk4):
	b L2
L3 (SSA Block: blk3):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	orr w8, wzr, #0x1
	cbz w8, #0x8 L3
L4 (SSA Block: blk4):
	b #-0x8 (L2)
L3 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "block_block_br", m: testcases.BlockBlockBr.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
L2 (SSA Block: blk1):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "if_without_else", m: testcases.IfWithoutElse.Module,
			// Note: The block of "b L4" seems redundant, but it is needed when we need to insert return value preparations.
			// So we cannot have the general optimization on this kind of redundant branch elimination before register allocations.
			// Instead, we can do it during the code generation phase where we actually resolve the label offsets.
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x3?, xzr
	cbnz w3?, L2
L3 (SSA Block: blk2):
	b L4
L2 (SSA Block: blk1):
L4 (SSA Block: blk3):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbnz w8, #0x8 (L2)
L3 (SSA Block: blk2):
	b #0x4 (L4)
L2 (SSA Block: blk1):
L4 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "if_else", m: testcases.IfElse.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x3?, xzr
	cbnz w3?, L2
L3 (SSA Block: blk2):
	ret
L2 (SSA Block: blk1):
L4 (SSA Block: blk3):
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbnz w8, #0x10 (L2)
L3 (SSA Block: blk2):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk1):
L4 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "single_predecessor_local_refs", m: testcases.SinglePredecessorLocalRefs.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x4?, xzr
	cbnz w4?, L2
L3 (SSA Block: blk2):
L4 (SSA Block: blk3):
	mov x2?, xzr
	mov x0, x2?
	ret
L2 (SSA Block: blk1):
	mov x3?, xzr
	mov x0, x3?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbnz w8, #0x14 (L2)
L3 (SSA Block: blk2):
L4 (SSA Block: blk3):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk1):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "multi_predecessor_local_ref", m: testcases.MultiPredecessorLocalRef.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	cbnz w2?, L2
L3 (SSA Block: blk2):
	mov x4?, x3?
	b L4
L2 (SSA Block: blk1):
	mov x4?, x2?
L4 (SSA Block: blk3):
	mov x0, x4?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	cbnz w2, #0xc (L2)
L3 (SSA Block: blk2):
	mov x0, x3
	b #0x8 (L4)
L2 (SSA Block: blk1):
	mov x0, x2
L4 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "reference_value_from_unsealed_block", m: testcases.ReferenceValueFromUnsealedBlock.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
L2 (SSA Block: blk1):
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "reference_value_from_unsealed_block2", m: testcases.ReferenceValueFromUnsealedBlock2.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
L2 (SSA Block: blk1):
	cbz w2?, (L3)
L4 (SSA Block: blk5):
	b L2
L3 (SSA Block: blk4):
L5 (SSA Block: blk3):
L6 (SSA Block: blk2):
	mov x3?, xzr
	mov x0, x3?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L2 (SSA Block: blk1):
	cbz w2, #0x8 L3
L4 (SSA Block: blk5):
	b #-0x4 (L2)
L3 (SSA Block: blk4):
L5 (SSA Block: blk3):
L6 (SSA Block: blk2):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "reference_value_from_unsealed_block3", m: testcases.ReferenceValueFromUnsealedBlock3.Module,
			// TODO: we should be able to invert cbnz in so that L2 can end with fallthrough. investigate builder.LayoutBlocks function.
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x2?
L2 (SSA Block: blk1):
	cbnz w3?, L4
	b L3
L4 (SSA Block: blk5):
	ret
L3 (SSA Block: blk4):
L5 (SSA Block: blk3):
	orr w3?, wzr, #0x1
	b L2
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, x2
L2 (SSA Block: blk1):
	cbnz w8, #0x8 (L4)
	b #0x10 (L3)
L4 (SSA Block: blk5):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L3 (SSA Block: blk4):
L5 (SSA Block: blk3):
	orr w8, wzr, #0x1
	b #-0x18 (L2)
`,
		},
		{
			name: "call", m: testcases.Call.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	str x1?, [x0?, #0x8]
	mov x0, x0?
	mov x1, x1?
	bl f1
	mov x2?, x0
	str x1?, [x0?, #0x8]
	mov x0, x0?
	mov x1, x1?
	mov x2, x2?
	movz w3?, #0x5, lsl 0
	mov x3, x3?
	bl f2
	mov x4?, x0
	str x1?, [x0?, #0x8]
	mov x0, x0?
	mov x1, x1?
	mov x2, x4?
	bl f3
	mov x5?, x0
	mov x6?, x1
	mov x1, x6?
	mov x0, x5?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x10
	orr x27, xzr, #0x10
	str x27, [sp, #-0x10]!
	mov x9, x0
	mov x8, x1
	str x8, [x9, #0x8]
	mov x0, x9
	mov x1, x8
	str x9, [sp, #0x10]
	str x8, [sp, #0x18]
	bl f1
	ldr x8, [sp, #0x18]
	ldr x9, [sp, #0x10]
	mov x2, x0
	str x8, [x9, #0x8]
	mov x0, x9
	mov x1, x8
	movz w3, #0x5, lsl 0
	str x9, [sp, #0x10]
	str x8, [sp, #0x18]
	bl f2
	ldr x8, [sp, #0x18]
	ldr x9, [sp, #0x10]
	mov x2, x0
	str x8, [x9, #0x8]
	mov x0, x9
	mov x1, x8
	bl f3
	add sp, sp, #0x10
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "call_many_params", m: testcases.CallManyParams.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	mov x2?, x2
	mov x3?, x3
	mov v4?.8b, v0.8b
	mov v5?.8b, v1.8b
	str x1?, [x0?, #0x8]
	sub sp, sp, #0xd0
	mov x0, x0?
	mov x1, x1?
	mov x2, x2?
	mov x3, x3?
	mov v0.8b, v4?.8b
	mov v1.8b, v5?.8b
	mov x4, x2?
	mov x5, x3?
	mov v2.8b, v4?.8b
	mov v3.8b, v5?.8b
	mov x6, x2?
	mov x7, x3?
	mov v4.8b, v4?.8b
	mov v5.8b, v5?.8b
	str w2?, [sp]
	str x3?, [sp, #0x8]
	mov v6.8b, v4?.8b
	mov v7.8b, v5?.8b
	str w2?, [sp, #0x10]
	str x3?, [sp, #0x18]
	str s4?, [sp, #0x20]
	str d5?, [sp, #0x28]
	str w2?, [sp, #0x30]
	str x3?, [sp, #0x38]
	str s4?, [sp, #0x40]
	str d5?, [sp, #0x48]
	str w2?, [sp, #0x50]
	str x3?, [sp, #0x58]
	str s4?, [sp, #0x60]
	str d5?, [sp, #0x68]
	str w2?, [sp, #0x70]
	str x3?, [sp, #0x78]
	str s4?, [sp, #0x80]
	str d5?, [sp, #0x88]
	str w2?, [sp, #0x90]
	str x3?, [sp, #0x98]
	str s4?, [sp, #0xa0]
	str d5?, [sp, #0xa8]
	str w2?, [sp, #0xb0]
	str x3?, [sp, #0xb8]
	str s4?, [sp, #0xc0]
	str d5?, [sp, #0xc8]
	bl f1
	add sp, sp, #0xd0
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, x2
	mov x9, x3
	mov v8.8b, v0.8b
	mov v9.8b, v1.8b
	str x1, [x0, #0x8]
	sub sp, sp, #0xd0
	mov x2, x8
	mov x3, x9
	mov v0.8b, v8.8b
	mov v1.8b, v9.8b
	mov x4, x8
	mov x5, x9
	mov v2.8b, v8.8b
	mov v3.8b, v9.8b
	mov x6, x8
	mov x7, x9
	mov v4.8b, v8.8b
	mov v5.8b, v9.8b
	str w8, [sp]
	str x9, [sp, #0x8]
	mov v6.8b, v8.8b
	mov v7.8b, v9.8b
	str w8, [sp, #0x10]
	str x9, [sp, #0x18]
	str s8, [sp, #0x20]
	str d9, [sp, #0x28]
	str w8, [sp, #0x30]
	str x9, [sp, #0x38]
	str s8, [sp, #0x40]
	str d9, [sp, #0x48]
	str w8, [sp, #0x50]
	str x9, [sp, #0x58]
	str s8, [sp, #0x60]
	str d9, [sp, #0x68]
	str w8, [sp, #0x70]
	str x9, [sp, #0x78]
	str s8, [sp, #0x80]
	str d9, [sp, #0x88]
	str w8, [sp, #0x90]
	str x9, [sp, #0x98]
	str s8, [sp, #0xa0]
	str d9, [sp, #0xa8]
	str w8, [sp, #0xb0]
	str x9, [sp, #0xb8]
	str s8, [sp, #0xc0]
	str d9, [sp, #0xc8]
	bl f1
	add sp, sp, #0xd0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "call_many_returns", m: testcases.CallManyReturns.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	mov x2?, x2
	mov x3?, x3
	mov v4?.8b, v0.8b
	mov v5?.8b, v1.8b
	str x1?, [x0?, #0x8]
	sub sp, sp, #0xc0
	mov x0, x0?
	mov x1, x1?
	mov x2, x2?
	mov x3, x3?
	mov v0.8b, v4?.8b
	mov v1.8b, v5?.8b
	bl f1
	mov x6?, x0
	mov x7?, x1
	mov v8?.8b, v0.8b
	mov v9?.8b, v1.8b
	mov x10?, x2
	mov x11?, x3
	mov v12?.8b, v2.8b
	mov v13?.8b, v3.8b
	mov x14?, x4
	mov x15?, x5
	mov v16?.8b, v4.8b
	mov v17?.8b, v5.8b
	mov x18?, x6
	mov x19?, x7
	mov v20?.8b, v6.8b
	mov v21?.8b, v7.8b
	ldr w22?, [sp]
	ldr x23?, [sp, #0x8]
	ldr s24?, [sp, #0x10]
	ldr d25?, [sp, #0x18]
	ldr w26?, [sp, #0x20]
	ldr x27?, [sp, #0x28]
	ldr s28?, [sp, #0x30]
	ldr d29?, [sp, #0x38]
	ldr w30?, [sp, #0x40]
	ldr x31?, [sp, #0x48]
	ldr s32?, [sp, #0x50]
	ldr d33?, [sp, #0x58]
	ldr w34?, [sp, #0x60]
	ldr x35?, [sp, #0x68]
	ldr s36?, [sp, #0x70]
	ldr d37?, [sp, #0x78]
	ldr w38?, [sp, #0x80]
	ldr x39?, [sp, #0x88]
	ldr s40?, [sp, #0x90]
	ldr d41?, [sp, #0x98]
	ldr w42?, [sp, #0xa0]
	ldr x43?, [sp, #0xa8]
	ldr s44?, [sp, #0xb0]
	ldr d45?, [sp, #0xb8]
	add sp, sp, #0xc0
	str d45?, [#ret_space, #0xb8]
	str s44?, [#ret_space, #0xb0]
	str x43?, [#ret_space, #0xa8]
	str w42?, [#ret_space, #0xa0]
	str d41?, [#ret_space, #0x98]
	str s40?, [#ret_space, #0x90]
	str x39?, [#ret_space, #0x88]
	str w38?, [#ret_space, #0x80]
	str d37?, [#ret_space, #0x78]
	str s36?, [#ret_space, #0x70]
	str x35?, [#ret_space, #0x68]
	str w34?, [#ret_space, #0x60]
	str d33?, [#ret_space, #0x58]
	str s32?, [#ret_space, #0x50]
	str x31?, [#ret_space, #0x48]
	str w30?, [#ret_space, #0x40]
	str d29?, [#ret_space, #0x38]
	str s28?, [#ret_space, #0x30]
	str x27?, [#ret_space, #0x28]
	str w26?, [#ret_space, #0x20]
	str d25?, [#ret_space, #0x18]
	str s24?, [#ret_space, #0x10]
	str x23?, [#ret_space, #0x8]
	str w22?, [#ret_space, #0x0]
	mov v7.8b, v21?.8b
	mov v6.8b, v20?.8b
	mov x7, x19?
	mov x6, x18?
	mov v5.8b, v17?.8b
	mov v4.8b, v16?.8b
	mov x5, x15?
	mov x4, x14?
	mov v3.8b, v13?.8b
	mov v2.8b, v12?.8b
	mov x3, x11?
	mov x2, x10?
	mov v1.8b, v9?.8b
	mov v0.8b, v8?.8b
	mov x1, x7?
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0xc0
	stp x30, x27, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	orr x27, xzr, #0x30
	str x27, [sp, #-0x10]!
	str x1, [x0, #0x8]
	sub sp, sp, #0xc0
	bl f1
	ldr w19, [sp]
	ldr x18, [sp, #0x8]
	ldr s19, [sp, #0x10]
	ldr d18, [sp, #0x18]
	ldr w17, [sp, #0x20]
	ldr x16, [sp, #0x28]
	ldr s17, [sp, #0x30]
	ldr d16, [sp, #0x38]
	ldr w15, [sp, #0x40]
	ldr x14, [sp, #0x48]
	ldr s15, [sp, #0x50]
	ldr d14, [sp, #0x58]
	ldr w13, [sp, #0x60]
	ldr x12, [sp, #0x68]
	ldr s13, [sp, #0x70]
	ldr d12, [sp, #0x78]
	ldr w11, [sp, #0x80]
	ldr x10, [sp, #0x88]
	ldr s11, [sp, #0x90]
	ldr d10, [sp, #0x98]
	ldr w9, [sp, #0xa0]
	ldr x8, [sp, #0xa8]
	ldr s9, [sp, #0xb0]
	ldr d8, [sp, #0xb8]
	add sp, sp, #0xc0
	str d8, [sp, #0x108]
	str s9, [sp, #0x100]
	str x8, [sp, #0xf8]
	str w9, [sp, #0xf0]
	str d10, [sp, #0xe8]
	str s11, [sp, #0xe0]
	str x10, [sp, #0xd8]
	str w11, [sp, #0xd0]
	str d12, [sp, #0xc8]
	str s13, [sp, #0xc0]
	str x12, [sp, #0xb8]
	str w13, [sp, #0xb0]
	str d14, [sp, #0xa8]
	str s15, [sp, #0xa0]
	str x14, [sp, #0x98]
	str w15, [sp, #0x90]
	str d16, [sp, #0x88]
	str s17, [sp, #0x80]
	str x16, [sp, #0x78]
	str w17, [sp, #0x70]
	str d18, [sp, #0x68]
	str s19, [sp, #0x60]
	str x18, [sp, #0x58]
	str w19, [sp, #0x50]
	add sp, sp, #0x10
	ldr q19, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "many_params_small_results", m: testcases.ManyParamsSmallResults.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x11?, x7
	ldr s20?, [#arg_space, #0x20]
	ldr d29?, [#arg_space, #0x68]
	mov v1.8b, v29?.8b
	mov v0.8b, v20?.8b
	mov x1, x11?
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	movz x27, #0xd0, lsl 0
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	ldr s0, [sp, #0x40]
	ldr d1, [sp, #0x88]
	mov x1, x7
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "samll_params_many_results", m: testcases.SmallParamsManyResults.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov v4?.8b, v0.8b
	mov v5?.8b, v1.8b
	str d5?, [#ret_space, #0xb8]
	str s4?, [#ret_space, #0xb0]
	str x3?, [#ret_space, #0xa8]
	str w2?, [#ret_space, #0xa0]
	str d5?, [#ret_space, #0x98]
	str s4?, [#ret_space, #0x90]
	str x3?, [#ret_space, #0x88]
	str w2?, [#ret_space, #0x80]
	str d5?, [#ret_space, #0x78]
	str s4?, [#ret_space, #0x70]
	str x3?, [#ret_space, #0x68]
	str w2?, [#ret_space, #0x60]
	str d5?, [#ret_space, #0x58]
	str s4?, [#ret_space, #0x50]
	str x3?, [#ret_space, #0x48]
	str w2?, [#ret_space, #0x40]
	str d5?, [#ret_space, #0x38]
	str s4?, [#ret_space, #0x30]
	str x3?, [#ret_space, #0x28]
	str w2?, [#ret_space, #0x20]
	str d5?, [#ret_space, #0x18]
	str s4?, [#ret_space, #0x10]
	str x3?, [#ret_space, #0x8]
	str w2?, [#ret_space, #0x0]
	mov v7.8b, v5?.8b
	mov v6.8b, v4?.8b
	mov x7, x3?
	mov x6, x2?
	mov v5.8b, v5?.8b
	mov v4.8b, v4?.8b
	mov x5, x3?
	mov x4, x2?
	mov v3.8b, v5?.8b
	mov v2.8b, v4?.8b
	mov x3, x3?
	mov x2, x2?
	mov v1.8b, v5?.8b
	mov v0.8b, v4?.8b
	mov x1, x3?
	mov x0, x2?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0xc0
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x0, x2
	mov x1, x3
	str d1, [sp, #0xd8]
	str s0, [sp, #0xd0]
	str x1, [sp, #0xc8]
	str w0, [sp, #0xc0]
	str d1, [sp, #0xb8]
	str s0, [sp, #0xb0]
	str x1, [sp, #0xa8]
	str w0, [sp, #0xa0]
	str d1, [sp, #0x98]
	str s0, [sp, #0x90]
	str x1, [sp, #0x88]
	str w0, [sp, #0x80]
	str d1, [sp, #0x78]
	str s0, [sp, #0x70]
	str x1, [sp, #0x68]
	str w0, [sp, #0x60]
	str d1, [sp, #0x58]
	str s0, [sp, #0x50]
	str x1, [sp, #0x48]
	str w0, [sp, #0x40]
	str d1, [sp, #0x38]
	str s0, [sp, #0x30]
	str x1, [sp, #0x28]
	str w0, [sp, #0x20]
	mov v7.8b, v1.8b
	mov v6.8b, v0.8b
	mov x7, x1
	mov x6, x0
	mov v5.8b, v1.8b
	mov v4.8b, v0.8b
	mov x5, x1
	mov x4, x0
	mov v3.8b, v1.8b
	mov v2.8b, v0.8b
	mov x3, x1
	mov x2, x0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "many_params_many_results", m: testcases.ManyParamsManyResults.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov v4?.8b, v0.8b
	mov v5?.8b, v1.8b
	mov x6?, x4
	mov x7?, x5
	mov v8?.8b, v2.8b
	mov v9?.8b, v3.8b
	mov x10?, x6
	mov x11?, x7
	mov v12?.8b, v4.8b
	mov v13?.8b, v5.8b
	ldr w14?, [#arg_space, #0x0]
	ldr x15?, [#arg_space, #0x8]
	mov v16?.8b, v6.8b
	mov v17?.8b, v7.8b
	ldr w18?, [#arg_space, #0x10]
	ldr x19?, [#arg_space, #0x18]
	ldr s20?, [#arg_space, #0x20]
	ldr d21?, [#arg_space, #0x28]
	ldr w22?, [#arg_space, #0x30]
	ldr x23?, [#arg_space, #0x38]
	ldr s24?, [#arg_space, #0x40]
	ldr d25?, [#arg_space, #0x48]
	ldr w26?, [#arg_space, #0x50]
	ldr x27?, [#arg_space, #0x58]
	ldr s28?, [#arg_space, #0x60]
	ldr d29?, [#arg_space, #0x68]
	ldr w30?, [#arg_space, #0x70]
	ldr x31?, [#arg_space, #0x78]
	ldr s32?, [#arg_space, #0x80]
	ldr d33?, [#arg_space, #0x88]
	ldr w34?, [#arg_space, #0x90]
	ldr x35?, [#arg_space, #0x98]
	ldr s36?, [#arg_space, #0xa0]
	ldr d37?, [#arg_space, #0xa8]
	ldr w38?, [#arg_space, #0xb0]
	ldr x39?, [#arg_space, #0xb8]
	ldr s40?, [#arg_space, #0xc0]
	ldr d41?, [#arg_space, #0xc8]
	str w2?, [#ret_space, #0xb8]
	str x3?, [#ret_space, #0xb0]
	str s4?, [#ret_space, #0xa8]
	str d5?, [#ret_space, #0xa0]
	str w6?, [#ret_space, #0x98]
	str x7?, [#ret_space, #0x90]
	str s8?, [#ret_space, #0x88]
	str d9?, [#ret_space, #0x80]
	str w10?, [#ret_space, #0x78]
	str x11?, [#ret_space, #0x70]
	str s12?, [#ret_space, #0x68]
	str d13?, [#ret_space, #0x60]
	str w14?, [#ret_space, #0x58]
	str x15?, [#ret_space, #0x50]
	str s16?, [#ret_space, #0x48]
	str d17?, [#ret_space, #0x40]
	str w18?, [#ret_space, #0x38]
	str x19?, [#ret_space, #0x30]
	str s20?, [#ret_space, #0x28]
	str d21?, [#ret_space, #0x20]
	str w22?, [#ret_space, #0x18]
	str x23?, [#ret_space, #0x10]
	str s24?, [#ret_space, #0x8]
	str d25?, [#ret_space, #0x0]
	mov x7, x26?
	mov x6, x27?
	mov v7.8b, v28?.8b
	mov v6.8b, v29?.8b
	mov x5, x30?
	mov x4, x31?
	mov v5.8b, v32?.8b
	mov v4.8b, v33?.8b
	mov x3, x34?
	mov x2, x35?
	mov v3.8b, v36?.8b
	mov v2.8b, v37?.8b
	mov x1, x38?
	mov x0, x39?
	mov v1.8b, v40?.8b
	mov v0.8b, v41?.8b
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	movz x27, #0x190, lsl 0
	stp x30, x27, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	orr x27, xzr, #0x30
	str x27, [sp, #-0x10]!
	mov x8, x2
	mov x9, x3
	mov v8.8b, v0.8b
	mov v9.8b, v1.8b
	mov x10, x4
	mov x11, x5
	mov v10.8b, v2.8b
	mov v11.8b, v3.8b
	mov x12, x6
	mov x19, x7
	mov v12.8b, v4.8b
	mov v13.8b, v5.8b
	ldr w18, [sp, #0x50]
	ldr x17, [sp, #0x58]
	mov v14.8b, v6.8b
	mov v19.8b, v7.8b
	ldr w16, [sp, #0x60]
	ldr x15, [sp, #0x68]
	ldr s18, [sp, #0x70]
	ldr d17, [sp, #0x78]
	ldr w14, [sp, #0x80]
	ldr x13, [sp, #0x88]
	ldr s16, [sp, #0x90]
	ldr d15, [sp, #0x98]
	ldr w7, [sp, #0xa0]
	ldr x6, [sp, #0xa8]
	ldr s7, [sp, #0xb0]
	ldr d6, [sp, #0xb8]
	ldr w5, [sp, #0xc0]
	ldr x4, [sp, #0xc8]
	ldr s5, [sp, #0xd0]
	ldr d4, [sp, #0xd8]
	ldr w3, [sp, #0xe0]
	ldr x2, [sp, #0xe8]
	ldr s3, [sp, #0xf0]
	ldr d2, [sp, #0xf8]
	ldr w1, [sp, #0x100]
	ldr x0, [sp, #0x108]
	ldr s1, [sp, #0x110]
	ldr d0, [sp, #0x118]
	str w8, [sp, #0x1d8]
	str x9, [sp, #0x1d0]
	str s8, [sp, #0x1c8]
	str d9, [sp, #0x1c0]
	str w10, [sp, #0x1b8]
	str x11, [sp, #0x1b0]
	str s10, [sp, #0x1a8]
	str d11, [sp, #0x1a0]
	str w12, [sp, #0x198]
	str x19, [sp, #0x190]
	str s12, [sp, #0x188]
	str d13, [sp, #0x180]
	str w18, [sp, #0x178]
	str x17, [sp, #0x170]
	str s14, [sp, #0x168]
	str d19, [sp, #0x160]
	str w16, [sp, #0x158]
	str x15, [sp, #0x150]
	str s18, [sp, #0x148]
	str d17, [sp, #0x140]
	str w14, [sp, #0x138]
	str x13, [sp, #0x130]
	str s16, [sp, #0x128]
	str d15, [sp, #0x120]
	add sp, sp, #0x10
	ldr q19, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer_comparisons",
			m:    testcases.IntegerComparisons.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov x4?, x4
	mov x5?, x5
	subs wzr, w2?, w3?
	cset x6?, eq
	subs xzr, x4?, x5?
	cset x7?, eq
	subs wzr, w2?, w3?
	cset x8?, ne
	subs xzr, x4?, x5?
	cset x9?, ne
	subs wzr, w2?, w3?
	cset x10?, lt
	subs xzr, x4?, x5?
	cset x11?, lt
	subs wzr, w2?, w3?
	cset x12?, lo
	subs xzr, x4?, x5?
	cset x13?, lo
	subs wzr, w2?, w3?
	cset x14?, gt
	subs xzr, x4?, x5?
	cset x15?, gt
	subs wzr, w2?, w3?
	cset x16?, hi
	subs xzr, x4?, x5?
	cset x17?, hi
	subs wzr, w2?, w3?
	cset x18?, le
	subs xzr, x4?, x5?
	cset x19?, le
	subs wzr, w2?, w3?
	cset x20?, ls
	subs xzr, x4?, x5?
	cset x21?, ls
	subs wzr, w2?, w3?
	cset x22?, ge
	subs xzr, x4?, x5?
	cset x23?, ge
	subs wzr, w2?, w3?
	cset x24?, hs
	subs xzr, x4?, x5?
	cset x25?, hs
	str w25?, [#ret_space, #0x58]
	str w24?, [#ret_space, #0x50]
	str w23?, [#ret_space, #0x48]
	str w22?, [#ret_space, #0x40]
	str w21?, [#ret_space, #0x38]
	str w20?, [#ret_space, #0x30]
	str w19?, [#ret_space, #0x28]
	str w18?, [#ret_space, #0x20]
	str w17?, [#ret_space, #0x18]
	str w16?, [#ret_space, #0x10]
	str w15?, [#ret_space, #0x8]
	str w14?, [#ret_space, #0x0]
	mov x7, x13?
	mov x6, x12?
	mov x5, x11?
	mov x4, x10?
	mov x3, x9?
	mov x2, x8?
	mov x1, x7?
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0x60
	stp x30, x27, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str x20, [sp, #-0x10]!
	str x21, [sp, #-0x10]!
	orr x27, xzr, #0x30
	str x27, [sp, #-0x10]!
	mov x8, x2
	mov x20, x3
	mov x19, x4
	mov x21, x5
	subs wzr, w8, w20
	cset x0, eq
	subs xzr, x19, x21
	cset x1, eq
	subs wzr, w8, w20
	cset x2, ne
	subs xzr, x19, x21
	cset x3, ne
	subs wzr, w8, w20
	cset x4, lt
	subs xzr, x19, x21
	cset x5, lt
	subs wzr, w8, w20
	cset x6, lo
	subs xzr, x19, x21
	cset x7, lo
	subs wzr, w8, w20
	cset x18, gt
	subs xzr, x19, x21
	cset x17, gt
	subs wzr, w8, w20
	cset x16, hi
	subs xzr, x19, x21
	cset x15, hi
	subs wzr, w8, w20
	cset x14, le
	subs xzr, x19, x21
	cset x13, le
	subs wzr, w8, w20
	cset x12, ls
	subs xzr, x19, x21
	cset x11, ls
	subs wzr, w8, w20
	cset x10, ge
	subs xzr, x19, x21
	cset x9, ge
	subs wzr, w8, w20
	cset x8, hs
	subs xzr, x19, x21
	cset x19, hs
	str w19, [sp, #0xa8]
	str w8, [sp, #0xa0]
	str w9, [sp, #0x98]
	str w10, [sp, #0x90]
	str w11, [sp, #0x88]
	str w12, [sp, #0x80]
	str w13, [sp, #0x78]
	str w14, [sp, #0x70]
	str w15, [sp, #0x68]
	str w16, [sp, #0x60]
	str w17, [sp, #0x58]
	str w18, [sp, #0x50]
	add sp, sp, #0x10
	ldr x21, [sp], #0x10
	ldr x20, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer_bitwise",
			m:    testcases.IntegerBitwise.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov x4?, x4
	mov x5?, x5
	and w6?, w2?, w3?
	orr w7?, w2?, w3?
	eor w8?, w2?, w3?
	ror w9?, w2?, w3?
	and x10?, x4?, x5?
	orr x11?, x4?, x5?
	eor x12?, x4?, x5?
	eor x15?, x4?, x5?, lsl #8
	sub x5?, xzr, x5?
	ror x16?, x4?, x5?
	ror x17?, x4?, x5?
	str x17?, [#ret_space, #0x8]
	str x16?, [#ret_space, #0x0]
	mov x7, x15?
	mov x6, x12?
	mov x5, x11?
	mov x4, x10?
	mov x3, x9?
	mov x2, x8?
	mov x1, x7?
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0x10
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x10, x2
	mov x8, x4
	mov x9, x5
	and w0, w10, w3
	orr w1, w10, w3
	eor w2, w10, w3
	ror w3, w10, w3
	and x4, x8, x9
	orr x5, x8, x9
	eor x6, x8, x9
	eor x7, x8, x9, lsl #8
	sub x9, xzr, x9
	ror x10, x8, x9
	ror x8, x8, x9
	str x8, [sp, #0x28]
	str x10, [sp, #0x20]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer_shift",
			m:    testcases.IntegerShift.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	mov x4?, x4
	mov x5?, x5
	lsl w6?, w2?, w3?
	lsl w8?, w2?, 0x1f
	lsl x9?, x4?, x5?
	lsl x11?, x4?, 0x20
	lsr w12?, w2?, w3?
	lsr w14?, w2?, 0x1f
	lsr x15?, x4?, x5?
	lsr x17?, x4?, 0x20
	asr w18?, w2?, w3?
	asr w20?, w2?, 0x1f
	asr x21?, x4?, x5?
	asr x23?, x4?, 0x20
	str x23?, [#ret_space, #0x18]
	str x21?, [#ret_space, #0x10]
	str w20?, [#ret_space, #0x8]
	str w18?, [#ret_space, #0x0]
	mov x7, x17?
	mov x6, x15?
	mov x5, x14?
	mov x4, x12?
	mov x3, x11?
	mov x2, x9?
	mov x1, x8?
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0x20
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x9, x2
	mov x10, x3
	mov x8, x4
	mov x11, x5
	lsl w0, w9, w10
	lsl w1, w9, 0x1f
	lsl x2, x8, x11
	lsl x3, x8, 0x20
	lsr w4, w9, w10
	lsr w5, w9, 0x1f
	lsr x6, x8, x11
	lsr x7, x8, 0x20
	asr w10, w9, w10
	asr w9, w9, 0x1f
	asr x11, x8, x11
	asr x8, x8, 0x20
	str x8, [sp, #0x38]
	str x11, [sp, #0x30]
	str w9, [sp, #0x28]
	str w10, [sp, #0x20]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer_extensions",
			m:    testcases.IntegerExtensions.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	sxtw x4?, w2?
	uxtw x5?, w2?
	sxtb x6?, w3?
	sxth x7?, w3?
	sxtw x8?, w3?
	sxtb w9?, w2?
	sxth w10?, w2?
	mov x6, x10?
	mov x5, x9?
	mov x4, x8?
	mov x3, x7?
	mov x2, x6?
	mov x1, x5?
	mov x0, x4?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, x2
	mov x9, x3
	sxtw x0, w8
	uxtw x1, w8
	sxtb x2, w9
	sxth x3, w9
	sxtw x4, w9
	sxtb w5, w8
	sxth w6, w8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer bit counts", m: testcases.IntegerBitCounts.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov x3?, x3
	clz w4?, w2?
	rbit w17?, w2?
	clz w5?, w17?
	ins v14?.d[0], x2?
	cnt v15?.16b, v14?.16b
	uaddlv h16?, v15?.8b
	mov x6?, v16?.d[0]
	clz x7?, x3?
	rbit x13?, x3?
	clz x8?, x13?
	ins v10?.d[0], x3?
	cnt v11?.16b, v10?.16b
	uaddlv h12?, v11?.8b
	mov x9?, v12?.d[0]
	mov x5, x9?
	mov x4, x8?
	mov x3, x7?
	mov x2, x6?
	mov x1, x5?
	mov x0, x4?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, x3
	clz w0, w2
	rbit w9, w2
	clz w1, w9
	ins v8.d[0], x2
	cnt v8.16b, v8.16b
	uaddlv h8, v8.8b
	mov x2, v8.d[0]
	clz x3, x8
	rbit x9, x8
	clz x4, x9
	ins v8.d[0], x8
	cnt v8.16b, v8.16b
	uaddlv h8, v8.8b
	mov x5, v8.d[0]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "float_comparisons",
			m:    testcases.FloatComparisons.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov v2?.8b, v0.8b
	mov v3?.8b, v1.8b
	mov v4?.8b, v2.8b
	mov v5?.8b, v3.8b
	fcmp s2?, s3?
	cset x6?, eq
	fcmp s2?, s3?
	cset x7?, ne
	fcmp s2?, s3?
	cset x8?, mi
	fcmp s2?, s3?
	cset x9?, gt
	fcmp s2?, s3?
	cset x10?, ls
	fcmp s2?, s3?
	cset x11?, ge
	fcmp d4?, d5?
	cset x12?, eq
	fcmp d4?, d5?
	cset x13?, ne
	fcmp d4?, d5?
	cset x14?, mi
	fcmp d4?, d5?
	cset x15?, gt
	fcmp d4?, d5?
	cset x16?, ls
	fcmp d4?, d5?
	cset x17?, ge
	str w17?, [#ret_space, #0x18]
	str w16?, [#ret_space, #0x10]
	str w15?, [#ret_space, #0x8]
	str w14?, [#ret_space, #0x0]
	mov x7, x13?
	mov x6, x12?
	mov x5, x11?
	mov x4, x10?
	mov x3, x9?
	mov x2, x8?
	mov x1, x7?
	mov x0, x6?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0x20
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	fcmp s0, s1
	cset x0, eq
	fcmp s0, s1
	cset x1, ne
	fcmp s0, s1
	cset x2, mi
	fcmp s0, s1
	cset x3, gt
	fcmp s0, s1
	cset x4, ls
	fcmp s0, s1
	cset x5, ge
	fcmp d2, d3
	cset x6, eq
	fcmp d2, d3
	cset x7, ne
	fcmp d2, d3
	cset x11, mi
	fcmp d2, d3
	cset x10, gt
	fcmp d2, d3
	cset x9, ls
	fcmp d2, d3
	cset x8, ge
	str w8, [sp, #0x38]
	str w9, [sp, #0x30]
	str w10, [sp, #0x28]
	str w11, [sp, #0x20]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "float_conversions",
			m:    testcases.FloatConversions.Module,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, x0
	msr fpsr, xzr
	fcvtzs x0, d0
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L17)
	fcmp d0, d0
	b.vc #0x34, (L16)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L16:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L17:
	msr fpsr, xzr
	fcvtzs x1, s1
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L15)
	fcmp s1, s1
	b.vc #0x34, (L14)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L14:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L15:
	msr fpsr, xzr
	fcvtzs w2, d0
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L13)
	fcmp d0, d0
	b.vc #0x34, (L12)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L12:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L13:
	msr fpsr, xzr
	fcvtzs w3, s1
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L11)
	fcmp s1, s1
	b.vc #0x34, (L10)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L10:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L11:
	msr fpsr, xzr
	fcvtzu x4, d0
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L9)
	fcmp d0, d0
	b.vc #0x34, (L8)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L8:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L9:
	msr fpsr, xzr
	fcvtzu x5, s1
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L7)
	fcmp s1, s1
	b.vc #0x34, (L6)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L6:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L7:
	msr fpsr, xzr
	fcvtzu w6, d0
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L5)
	fcmp d0, d0
	b.vc #0x34, (L4)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L4:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L5:
	msr fpsr, xzr
	fcvtzu w7, s1
	mrs x9 fpsr
	subs xzr, x9, #0x1
	b.ne #0x6c, (L3)
	fcmp s1, s1
	b.vc #0x34, (L2)
	movz x9, #0xc, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L2:
	movz x9, #0xb, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L3:
	fcvt s0, d0
	fcvt d1, s1
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "nontrapping_float_conversions",
			m:    testcases.NonTrappingFloatConversions.Module,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	fcvtzs x0, d0
	fcvtzs x1, s1
	fcvtzs w2, d0
	fcvtzs w3, s1
	fcvtzu x4, d0
	fcvtzu x5, s1
	fcvtzu w6, d0
	fcvtzu w7, s1
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "many_middle_values",
			m:    testcases.ManyMiddleValues.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	mov v3?.8b, v0.8b
	orr w161?, wzr, #0x1
	madd w5?, w2?, w161?, wzr
	orr w160?, wzr, #0x2
	madd w7?, w2?, w160?, wzr
	orr w159?, wzr, #0x3
	madd w9?, w2?, w159?, wzr
	orr w158?, wzr, #0x4
	madd w11?, w2?, w158?, wzr
	movz w157?, #0x5, lsl 0
	madd w13?, w2?, w157?, wzr
	orr w156?, wzr, #0x6
	madd w15?, w2?, w156?, wzr
	orr w155?, wzr, #0x7
	madd w17?, w2?, w155?, wzr
	orr w154?, wzr, #0x8
	madd w19?, w2?, w154?, wzr
	movz w153?, #0x9, lsl 0
	madd w21?, w2?, w153?, wzr
	movz w152?, #0xa, lsl 0
	madd w23?, w2?, w152?, wzr
	movz w151?, #0xb, lsl 0
	madd w25?, w2?, w151?, wzr
	orr w150?, wzr, #0xc
	madd w27?, w2?, w150?, wzr
	movz w149?, #0xd, lsl 0
	madd w29?, w2?, w149?, wzr
	orr w148?, wzr, #0xe
	madd w31?, w2?, w148?, wzr
	orr w147?, wzr, #0xf
	madd w33?, w2?, w147?, wzr
	orr w146?, wzr, #0x10
	madd w35?, w2?, w146?, wzr
	movz w145?, #0x11, lsl 0
	madd w37?, w2?, w145?, wzr
	movz w144?, #0x12, lsl 0
	madd w39?, w2?, w144?, wzr
	movz w143?, #0x13, lsl 0
	madd w41?, w2?, w143?, wzr
	movz w142?, #0x14, lsl 0
	madd w43?, w2?, w142?, wzr
	add w44?, w41?, w43?
	add w45?, w39?, w44?
	add w46?, w37?, w45?
	add w47?, w35?, w46?
	add w48?, w33?, w47?
	add w49?, w31?, w48?
	add w50?, w29?, w49?
	add w51?, w27?, w50?
	add w52?, w25?, w51?
	add w53?, w23?, w52?
	add w54?, w21?, w53?
	add w55?, w19?, w54?
	add w56?, w17?, w55?
	add w57?, w15?, w56?
	add w58?, w13?, w57?
	add w59?, w11?, w58?
	add w60?, w9?, w59?
	add w61?, w7?, w60?
	add w62?, w5?, w61?
	ldr s141?, #8; b 8; data.f32 1.000000
	fmul s64?, s3?, s141?
	ldr s140?, #8; b 8; data.f32 2.000000
	fmul s66?, s3?, s140?
	ldr s139?, #8; b 8; data.f32 3.000000
	fmul s68?, s3?, s139?
	ldr s138?, #8; b 8; data.f32 4.000000
	fmul s70?, s3?, s138?
	ldr s137?, #8; b 8; data.f32 5.000000
	fmul s72?, s3?, s137?
	ldr s136?, #8; b 8; data.f32 6.000000
	fmul s74?, s3?, s136?
	ldr s135?, #8; b 8; data.f32 7.000000
	fmul s76?, s3?, s135?
	ldr s134?, #8; b 8; data.f32 8.000000
	fmul s78?, s3?, s134?
	ldr s133?, #8; b 8; data.f32 9.000000
	fmul s80?, s3?, s133?
	ldr s132?, #8; b 8; data.f32 10.000000
	fmul s82?, s3?, s132?
	ldr s131?, #8; b 8; data.f32 11.000000
	fmul s84?, s3?, s131?
	ldr s130?, #8; b 8; data.f32 12.000000
	fmul s86?, s3?, s130?
	ldr s129?, #8; b 8; data.f32 13.000000
	fmul s88?, s3?, s129?
	ldr s128?, #8; b 8; data.f32 14.000000
	fmul s90?, s3?, s128?
	ldr s127?, #8; b 8; data.f32 15.000000
	fmul s92?, s3?, s127?
	ldr s126?, #8; b 8; data.f32 16.000000
	fmul s94?, s3?, s126?
	ldr s125?, #8; b 8; data.f32 17.000000
	fmul s96?, s3?, s125?
	ldr s124?, #8; b 8; data.f32 18.000000
	fmul s98?, s3?, s124?
	ldr s123?, #8; b 8; data.f32 19.000000
	fmul s100?, s3?, s123?
	ldr s122?, #8; b 8; data.f32 20.000000
	fmul s102?, s3?, s122?
	fadd s103?, s100?, s102?
	fadd s104?, s98?, s103?
	fadd s105?, s96?, s104?
	fadd s106?, s94?, s105?
	fadd s107?, s92?, s106?
	fadd s108?, s90?, s107?
	fadd s109?, s88?, s108?
	fadd s110?, s86?, s109?
	fadd s111?, s84?, s110?
	fadd s112?, s82?, s111?
	fadd s113?, s80?, s112?
	fadd s114?, s78?, s113?
	fadd s115?, s76?, s114?
	fadd s116?, s74?, s115?
	fadd s117?, s72?, s116?
	fadd s118?, s70?, s117?
	fadd s119?, s68?, s118?
	fadd s120?, s66?, s119?
	fadd s121?, s64?, s120?
	mov v0.8b, v121?.8b
	mov x0, x62?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str x20, [sp, #-0x10]!
	str x21, [sp, #-0x10]!
	str x22, [sp, #-0x10]!
	str x23, [sp, #-0x10]!
	str x24, [sp, #-0x10]!
	str x25, [sp, #-0x10]!
	str x26, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	str q20, [sp, #-0x10]!
	str q21, [sp, #-0x10]!
	str q22, [sp, #-0x10]!
	str q23, [sp, #-0x10]!
	str q24, [sp, #-0x10]!
	str q25, [sp, #-0x10]!
	str q26, [sp, #-0x10]!
	str q27, [sp, #-0x10]!
	movz x27, #0x120, lsl 0
	str x27, [sp, #-0x10]!
	orr w8, wzr, #0x1
	madd w8, w2, w8, wzr
	orr w9, wzr, #0x2
	madd w9, w2, w9, wzr
	orr w10, wzr, #0x3
	madd w10, w2, w10, wzr
	orr w11, wzr, #0x4
	madd w11, w2, w11, wzr
	movz w12, #0x5, lsl 0
	madd w12, w2, w12, wzr
	orr w13, wzr, #0x6
	madd w13, w2, w13, wzr
	orr w14, wzr, #0x7
	madd w14, w2, w14, wzr
	orr w15, wzr, #0x8
	madd w15, w2, w15, wzr
	movz w16, #0x9, lsl 0
	madd w16, w2, w16, wzr
	movz w17, #0xa, lsl 0
	madd w17, w2, w17, wzr
	movz w18, #0xb, lsl 0
	madd w18, w2, w18, wzr
	orr w19, wzr, #0xc
	madd w19, w2, w19, wzr
	movz w20, #0xd, lsl 0
	madd w20, w2, w20, wzr
	orr w21, wzr, #0xe
	madd w21, w2, w21, wzr
	orr w22, wzr, #0xf
	madd w22, w2, w22, wzr
	orr w23, wzr, #0x10
	madd w23, w2, w23, wzr
	movz w24, #0x11, lsl 0
	madd w24, w2, w24, wzr
	movz w25, #0x12, lsl 0
	madd w25, w2, w25, wzr
	movz w26, #0x13, lsl 0
	madd w26, w2, w26, wzr
	movz w29, #0x14, lsl 0
	madd w29, w2, w29, wzr
	add w26, w26, w29
	add w25, w25, w26
	add w24, w24, w25
	add w23, w23, w24
	add w22, w22, w23
	add w21, w21, w22
	add w20, w20, w21
	add w19, w19, w20
	add w18, w18, w19
	add w17, w17, w18
	add w16, w16, w17
	add w15, w15, w16
	add w14, w14, w15
	add w13, w13, w14
	add w12, w12, w13
	add w11, w11, w12
	add w10, w10, w11
	add w9, w9, w10
	add w0, w8, w9
	ldr s8, #8; b 8; data.f32 1.000000
	fmul s8, s0, s8
	ldr s9, #8; b 8; data.f32 2.000000
	fmul s9, s0, s9
	ldr s10, #8; b 8; data.f32 3.000000
	fmul s10, s0, s10
	ldr s11, #8; b 8; data.f32 4.000000
	fmul s11, s0, s11
	ldr s12, #8; b 8; data.f32 5.000000
	fmul s12, s0, s12
	ldr s13, #8; b 8; data.f32 6.000000
	fmul s13, s0, s13
	ldr s14, #8; b 8; data.f32 7.000000
	fmul s14, s0, s14
	ldr s15, #8; b 8; data.f32 8.000000
	fmul s15, s0, s15
	ldr s16, #8; b 8; data.f32 9.000000
	fmul s16, s0, s16
	ldr s17, #8; b 8; data.f32 10.000000
	fmul s17, s0, s17
	ldr s18, #8; b 8; data.f32 11.000000
	fmul s18, s0, s18
	ldr s19, #8; b 8; data.f32 12.000000
	fmul s19, s0, s19
	ldr s20, #8; b 8; data.f32 13.000000
	fmul s20, s0, s20
	ldr s21, #8; b 8; data.f32 14.000000
	fmul s21, s0, s21
	ldr s22, #8; b 8; data.f32 15.000000
	fmul s22, s0, s22
	ldr s23, #8; b 8; data.f32 16.000000
	fmul s23, s0, s23
	ldr s24, #8; b 8; data.f32 17.000000
	fmul s24, s0, s24
	ldr s25, #8; b 8; data.f32 18.000000
	fmul s25, s0, s25
	ldr s26, #8; b 8; data.f32 19.000000
	fmul s26, s0, s26
	ldr s27, #8; b 8; data.f32 20.000000
	fmul s27, s0, s27
	fadd s26, s26, s27
	fadd s25, s25, s26
	fadd s24, s24, s25
	fadd s23, s23, s24
	fadd s22, s22, s23
	fadd s21, s21, s22
	fadd s20, s20, s21
	fadd s19, s19, s20
	fadd s18, s18, s19
	fadd s17, s17, s18
	fadd s16, s16, s17
	fadd s15, s15, s16
	fadd s14, s14, s15
	fadd s13, s13, s14
	fadd s12, s12, s13
	fadd s11, s11, s12
	fadd s10, s10, s11
	fadd s9, s9, s10
	fadd s0, s8, s9
	add sp, sp, #0x10
	ldr q27, [sp], #0x10
	ldr q26, [sp], #0x10
	ldr q25, [sp], #0x10
	ldr q24, [sp], #0x10
	ldr q23, [sp], #0x10
	ldr q22, [sp], #0x10
	ldr q21, [sp], #0x10
	ldr q20, [sp], #0x10
	ldr q19, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x26, [sp], #0x10
	ldr x25, [sp], #0x10
	ldr x24, [sp], #0x10
	ldr x23, [sp], #0x10
	ldr x22, [sp], #0x10
	ldr x21, [sp], #0x10
	ldr x20, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "imported_function_call", m: testcases.ImportedFunctionCall.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	mov x2?, x2
	str x1?, [x0?, #0x8]
	ldr x3?, [x1?, #0x8]
	ldr x4?, [x1?, #0x10]
	mov x0, x0?
	mov x1, x4?
	mov x2, x2?
	mov x3, x2?
	bl x3?
	mov x5?, x0
	mov x0, x5?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x3, x2
	str x1, [x0, #0x8]
	ldr x8, [x1, #0x8]
	ldr x1, [x1, #0x10]
	mov x2, x3
	bl x8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "memory_load_basic", m: testcases.MemoryLoadBasic.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	mov x2?, x2
	uxtw x4?, w2?
	ldr w5?, [x1?, #0x10]
	add x6?, x4?, #0x4
	subs xzr, x5?, x6?
	b.hs L2
	movz x12?, #0x4, lsl 0
	str w12?, [x0?]
	mov x13?, sp
	str x13?, [x0?, #0x38]
	adr x14?, #0x0
	str x14?, [x0?, #0x30]
	exit_sequence x0?
L2:
	ldr x8?, [x1?, #0x8]
	add x11?, x8?, x4?
	ldr w10?, [x11?]
	mov x0, x10?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	uxtw x8, w2
	ldr w10, [x1, #0x10]
	add x9, x8, #0x4
	subs xzr, x10, x9
	b.hs #0x34, (L2)
	movz x9, #0x4, lsl 0
	str w9, [x0]
	mov x9, sp
	str x9, [x0, #0x38]
	adr x9, #0x0
	str x9, [x0, #0x30]
	exit_sequence x0
L2:
	ldr x9, [x1, #0x8]
	add x8, x9, x8
	ldr w0, [x8]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "memory_stores", m: testcases.MemoryStores.Module,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	uxtw x10, w8
	ldr w8, [x1, #0x10]
	add x9, x10, #0x4
	subs xzr, x8, x9
	b.hs #0x34, (L10)
	movz x9, #0x4, lsl 0
	str w9, [x0]
	mov x9, sp
	str x9, [x0, #0x38]
	adr x9, #0x0
	str x9, [x0, #0x30]
	exit_sequence x0
L10:
	ldr x9, [x1, #0x8]
	add x10, x9, x10
	str w2, [x10]
	orr w10, wzr, #0x8
	uxtw x10, w10
	add x11, x10, #0x8
	subs xzr, x8, x11
	b.hs #0x34, (L9)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L9:
	add x10, x9, x10
	str x3, [x10]
	orr w10, wzr, #0x10
	uxtw x10, w10
	add x11, x10, #0x4
	subs xzr, x8, x11
	b.hs #0x34, (L8)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L8:
	add x10, x9, x10
	str s0, [x10]
	orr w10, wzr, #0x18
	uxtw x10, w10
	add x11, x10, #0x8
	subs xzr, x8, x11
	b.hs #0x34, (L7)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L7:
	add x10, x9, x10
	str d1, [x10]
	orr w10, wzr, #0x20
	uxtw x10, w10
	add x11, x10, #0x1
	subs xzr, x8, x11
	b.hs #0x34, (L6)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L6:
	add x10, x9, x10
	strb w2, [x10]
	movz w10, #0x28, lsl 0
	uxtw x10, w10
	add x11, x10, #0x2
	subs xzr, x8, x11
	b.hs #0x34, (L5)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L5:
	add x10, x9, x10
	strh w2, [x10]
	orr w10, wzr, #0x30
	uxtw x10, w10
	add x11, x10, #0x1
	subs xzr, x8, x11
	b.hs #0x34, (L4)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L4:
	add x10, x9, x10
	strb w3, [x10]
	orr w10, wzr, #0x38
	uxtw x10, w10
	add x11, x10, #0x2
	subs xzr, x8, x11
	b.hs #0x34, (L3)
	movz x11, #0x4, lsl 0
	str w11, [x0]
	mov x11, sp
	str x11, [x0, #0x38]
	adr x11, #0x0
	str x11, [x0, #0x30]
	exit_sequence x0
L3:
	add x10, x9, x10
	strh w3, [x10]
	orr w10, wzr, #0x40
	uxtw x10, w10
	add x11, x10, #0x4
	subs xzr, x8, x11
	b.hs #0x34, (L2)
	movz x8, #0x4, lsl 0
	str w8, [x0]
	mov x8, sp
	str x8, [x0, #0x38]
	adr x8, #0x0
	str x8, [x0, #0x30]
	exit_sequence x0
L2:
	add x8, x9, x10
	str w3, [x8]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "memory_loads", m: testcases.MemoryLoads.Module,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	orr x27, xzr, #0x80
	stp x30, x27, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str x20, [sp, #-0x10]!
	str x21, [sp, #-0x10]!
	str x22, [sp, #-0x10]!
	str x23, [sp, #-0x10]!
	str x24, [sp, #-0x10]!
	str x25, [sp, #-0x10]!
	str x26, [sp, #-0x10]!
	orr x27, xzr, #0x80
	str x27, [sp, #-0x10]!
	mov x8, x0
	mov x24, x2
	uxtw x11, w24
	ldr w9, [x1, #0x10]
	add x10, x11, #0x4
	subs xzr, x9, x10
	b.hs #0x34, (L29)
	movz x10, #0x4, lsl 0
	str w10, [x8]
	mov x10, sp
	str x10, [x8, #0x38]
	adr x10, #0x0
	str x10, [x8, #0x30]
	exit_sequence x8
L29:
	ldr x10, [x1, #0x8]
	add x11, x10, x11
	ldr w0, [x11]
	uxtw x11, w24
	add x12, x11, #0x8
	subs xzr, x9, x12
	b.hs #0x34, (L28)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L28:
	add x11, x10, x11
	ldr x1, [x11]
	uxtw x11, w24
	add x12, x11, #0x4
	subs xzr, x9, x12
	b.hs #0x34, (L27)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L27:
	add x11, x10, x11
	ldr s0, [x11]
	uxtw x11, w24
	add x12, x11, #0x8
	subs xzr, x9, x12
	b.hs #0x34, (L26)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L26:
	add x11, x10, x11
	ldr d1, [x11]
	uxtw x11, w24
	add x12, x11, #0x13
	subs xzr, x9, x12
	b.hs #0x34, (L25)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L25:
	add x11, x10, x11
	ldr w2, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x17
	subs xzr, x9, x12
	b.hs #0x34, (L24)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L24:
	add x11, x10, x11
	ldr x3, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x13
	subs xzr, x9, x12
	b.hs #0x34, (L23)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L23:
	add x11, x10, x11
	ldr s2, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x17
	subs xzr, x9, x12
	b.hs #0x34, (L22)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L22:
	add x11, x10, x11
	ldr d3, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x1
	subs xzr, x9, x12
	b.hs #0x34, (L21)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L21:
	add x11, x10, x11
	ldrsb w4, [x11]
	uxtw x11, w24
	add x12, x11, #0x10
	subs xzr, x9, x12
	b.hs #0x34, (L20)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L20:
	add x11, x10, x11
	ldrsb w5, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x1
	subs xzr, x9, x12
	b.hs #0x34, (L19)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L19:
	add x11, x10, x11
	ldrb w6, [x11]
	uxtw x11, w24
	add x12, x11, #0x10
	subs xzr, x9, x12
	b.hs #0x34, (L18)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L18:
	add x11, x10, x11
	ldrb w7, [x11, #0xf]
	uxtw x11, w24
	add x12, x11, #0x2
	subs xzr, x9, x12
	b.hs #0x34, (L17)
	movz x12, #0x4, lsl 0
	str w12, [x8]
	mov x12, sp
	str x12, [x8, #0x38]
	adr x12, #0x0
	str x12, [x8, #0x30]
	exit_sequence x8
L17:
	add x11, x10, x11
	ldrsh w11, [x11]
	uxtw x12, w24
	add x13, x12, #0x11
	subs xzr, x9, x13
	b.hs #0x34, (L16)
	movz x13, #0x4, lsl 0
	str w13, [x8]
	mov x13, sp
	str x13, [x8, #0x38]
	adr x13, #0x0
	str x13, [x8, #0x30]
	exit_sequence x8
L16:
	add x12, x10, x12
	ldrsh w12, [x12, #0xf]
	uxtw x13, w24
	add x14, x13, #0x2
	subs xzr, x9, x14
	b.hs #0x34, (L15)
	movz x14, #0x4, lsl 0
	str w14, [x8]
	mov x14, sp
	str x14, [x8, #0x38]
	adr x14, #0x0
	str x14, [x8, #0x30]
	exit_sequence x8
L15:
	add x13, x10, x13
	ldrh w13, [x13]
	uxtw x14, w24
	add x15, x14, #0x11
	subs xzr, x9, x15
	b.hs #0x34, (L14)
	movz x15, #0x4, lsl 0
	str w15, [x8]
	mov x15, sp
	str x15, [x8, #0x38]
	adr x15, #0x0
	str x15, [x8, #0x30]
	exit_sequence x8
L14:
	add x14, x10, x14
	ldrh w14, [x14, #0xf]
	uxtw x15, w24
	add x16, x15, #0x1
	subs xzr, x9, x16
	b.hs #0x34, (L13)
	movz x16, #0x4, lsl 0
	str w16, [x8]
	mov x16, sp
	str x16, [x8, #0x38]
	adr x16, #0x0
	str x16, [x8, #0x30]
	exit_sequence x8
L13:
	add x15, x10, x15
	ldrsb w15, [x15]
	uxtw x16, w24
	add x17, x16, #0x10
	subs xzr, x9, x17
	b.hs #0x34, (L12)
	movz x17, #0x4, lsl 0
	str w17, [x8]
	mov x17, sp
	str x17, [x8, #0x38]
	adr x17, #0x0
	str x17, [x8, #0x30]
	exit_sequence x8
L12:
	add x16, x10, x16
	ldrsb w16, [x16, #0xf]
	uxtw x17, w24
	add x18, x17, #0x1
	subs xzr, x9, x18
	b.hs #0x34, (L11)
	movz x18, #0x4, lsl 0
	str w18, [x8]
	mov x18, sp
	str x18, [x8, #0x38]
	adr x18, #0x0
	str x18, [x8, #0x30]
	exit_sequence x8
L11:
	add x17, x10, x17
	ldrb w17, [x17]
	uxtw x18, w24
	add x19, x18, #0x10
	subs xzr, x9, x19
	b.hs #0x34, (L10)
	movz x19, #0x4, lsl 0
	str w19, [x8]
	mov x19, sp
	str x19, [x8, #0x38]
	adr x19, #0x0
	str x19, [x8, #0x30]
	exit_sequence x8
L10:
	add x18, x10, x18
	ldrb w18, [x18, #0xf]
	uxtw x19, w24
	add x20, x19, #0x2
	subs xzr, x9, x20
	b.hs #0x34, (L9)
	movz x20, #0x4, lsl 0
	str w20, [x8]
	mov x20, sp
	str x20, [x8, #0x38]
	adr x20, #0x0
	str x20, [x8, #0x30]
	exit_sequence x8
L9:
	add x19, x10, x19
	ldrsh w19, [x19]
	uxtw x20, w24
	add x21, x20, #0x11
	subs xzr, x9, x21
	b.hs #0x34, (L8)
	movz x21, #0x4, lsl 0
	str w21, [x8]
	mov x21, sp
	str x21, [x8, #0x38]
	adr x21, #0x0
	str x21, [x8, #0x30]
	exit_sequence x8
L8:
	add x20, x10, x20
	ldrsh w20, [x20, #0xf]
	uxtw x21, w24
	add x22, x21, #0x2
	subs xzr, x9, x22
	b.hs #0x34, (L7)
	movz x22, #0x4, lsl 0
	str w22, [x8]
	mov x22, sp
	str x22, [x8, #0x38]
	adr x22, #0x0
	str x22, [x8, #0x30]
	exit_sequence x8
L7:
	add x21, x10, x21
	ldrh w21, [x21]
	uxtw x22, w24
	add x23, x22, #0x11
	subs xzr, x9, x23
	b.hs #0x34, (L6)
	movz x23, #0x4, lsl 0
	str w23, [x8]
	mov x23, sp
	str x23, [x8, #0x38]
	adr x23, #0x0
	str x23, [x8, #0x30]
	exit_sequence x8
L6:
	add x22, x10, x22
	ldrh w22, [x22, #0xf]
	uxtw x23, w24
	add x25, x23, #0x4
	subs xzr, x9, x25
	b.hs #0x34, (L5)
	movz x25, #0x4, lsl 0
	str w25, [x8]
	mov x25, sp
	str x25, [x8, #0x38]
	adr x25, #0x0
	str x25, [x8, #0x30]
	exit_sequence x8
L5:
	add x23, x10, x23
	ldrs w23, [x23]
	uxtw x25, w24
	add x26, x25, #0x13
	subs xzr, x9, x26
	b.hs #0x34, (L4)
	movz x26, #0x4, lsl 0
	str w26, [x8]
	mov x26, sp
	str x26, [x8, #0x38]
	adr x26, #0x0
	str x26, [x8, #0x30]
	exit_sequence x8
L4:
	add x25, x10, x25
	ldrs w25, [x25, #0xf]
	uxtw x26, w24
	add x29, x26, #0x4
	subs xzr, x9, x29
	b.hs #0x34, (L3)
	movz x29, #0x4, lsl 0
	str w29, [x8]
	mov x29, sp
	str x29, [x8, #0x38]
	adr x29, #0x0
	str x29, [x8, #0x30]
	exit_sequence x8
L3:
	add x26, x10, x26
	ldr w26, [x26]
	uxtw x24, w24
	add x29, x24, #0x13
	subs xzr, x9, x29
	b.hs #0x34, (L2)
	movz x9, #0x4, lsl 0
	str w9, [x8]
	mov x9, sp
	str x9, [x8, #0x38]
	adr x9, #0x0
	str x9, [x8, #0x30]
	exit_sequence x8
L2:
	add x8, x10, x24
	ldr w8, [x8, #0xf]
	str x8, [sp, #0x118]
	str x26, [sp, #0x110]
	str x25, [sp, #0x108]
	str x23, [sp, #0x100]
	str x22, [sp, #0xf8]
	str x21, [sp, #0xf0]
	str x20, [sp, #0xe8]
	str x19, [sp, #0xe0]
	str x18, [sp, #0xd8]
	str x17, [sp, #0xd0]
	str x16, [sp, #0xc8]
	str x15, [sp, #0xc0]
	str w14, [sp, #0xb8]
	str w13, [sp, #0xb0]
	str w12, [sp, #0xa8]
	str w11, [sp, #0xa0]
	add sp, sp, #0x10
	ldr x26, [sp], #0x10
	ldr x25, [sp], #0x10
	ldr x24, [sp], #0x10
	ldr x23, [sp], #0x10
	ldr x22, [sp], #0x10
	ldr x21, [sp], #0x10
	ldr x20, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "globals_mutable[0]",
			m:    testcases.GlobalsMutable.Module,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x0?, x0
	mov x1?, x1
	ldr x2?, [x1?, #0x8]
	ldr w3?, [x2?, #0x8]
	ldr x4?, [x1?, #0x10]
	ldr x5?, [x4?, #0x8]
	ldr x6?, [x1?, #0x18]
	ldr s7?, [x6?, #0x8]
	ldr x8?, [x1?, #0x20]
	ldr d9?, [x8?, #0x8]
	str x1?, [x0?, #0x8]
	mov x0, x0?
	mov x1, x1?
	bl f1
	ldr x10?, [x1?, #0x8]
	ldr w11?, [x10?, #0x8]
	ldr x12?, [x1?, #0x10]
	ldr x13?, [x12?, #0x8]
	ldr x14?, [x1?, #0x18]
	ldr s15?, [x14?, #0x8]
	ldr x16?, [x1?, #0x20]
	ldr d17?, [x16?, #0x8]
	mov v3.8b, v17?.8b
	mov v2.8b, v15?.8b
	mov x3, x13?
	mov x2, x11?
	mov v1.8b, v9?.8b
	mov v0.8b, v7?.8b
	mov x1, x5?
	mov x0, x3?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x20
	orr x27, xzr, #0x20
	str x27, [sp, #-0x10]!
	mov x10, x1
	ldr x8, [x10, #0x8]
	ldr w8, [x8, #0x8]
	ldr x9, [x10, #0x10]
	ldr x9, [x9, #0x8]
	ldr x11, [x10, #0x18]
	ldr s0, [x11, #0x8]
	ldr x11, [x10, #0x20]
	ldr d1, [x11, #0x8]
	str x10, [x0, #0x8]
	mov x1, x10
	str x10, [sp, #0x10]
	str w8, [sp, #0x18]
	str x9, [sp, #0x1c]
	str s0, [sp, #0x24]
	str d1, [sp, #0x28]
	bl f1
	ldr d1, [sp, #0x28]
	ldr s0, [sp, #0x24]
	ldr x9, [sp, #0x1c]
	ldr w8, [sp, #0x18]
	ldr x10, [sp, #0x10]
	ldr x11, [x10, #0x8]
	ldr w2, [x11, #0x8]
	ldr x11, [x10, #0x10]
	ldr x3, [x11, #0x8]
	ldr x11, [x10, #0x18]
	ldr s2, [x11, #0x8]
	ldr x10, [x10, #0x20]
	ldr d3, [x10, #0x8]
	mov x1, x9
	mov x0, x8
	add sp, sp, #0x10
	add sp, sp, #0x20
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name:        "globals_mutable[1]",
			m:           testcases.GlobalsMutable.Module,
			targetIndex: 1,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x1?, x1
	ldr x3?, [x1?, #0x8]
	orr w13?, wzr, #0x1
	str w13?, [x3?, #0x8]
	ldr x5?, [x1?, #0x10]
	orr x12?, xzr, #0x2
	str x12?, [x5?, #0x8]
	ldr x7?, [x1?, #0x18]
	ldr s11?, #8; b 8; data.f32 3.000000
	str s11?, [x7?, #0x8]
	ldr x9?, [x1?, #0x20]
	ldr d10?, #8; b 16; data.f64 4.000000
	str d10?, [x9?, #0x8]
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	ldr x9, [x1, #0x8]
	orr w8, wzr, #0x1
	str w8, [x9, #0x8]
	ldr x9, [x1, #0x10]
	orr x8, xzr, #0x2
	str x8, [x9, #0x8]
	ldr x8, [x1, #0x18]
	ldr s8, #8; b 8; data.f32 3.000000
	str s8, [x8, #0x8]
	ldr x8, [x1, #0x20]
	ldr d8, #8; b 16; data.f64 4.000000
	str d8, [x8, #0x8]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name:        "br_table",
			m:           testcases.BrTable.Module,
			targetIndex: 0,
			afterLoweringARM64: `
L1 (SSA Block: blk0):
	mov x2?, x2
	orr w9?, wzr, #0x6
	subs wzr, w2?, w9?
	csel w10?, w9?, w2?, hs
	br_table_sequence x10?, [L2, L3, L4, L5, L6, L7, L8]
L2 (SSA Block: blk7):
	b L9
L3 (SSA Block: blk8):
L10 (SSA Block: blk5):
	orr w3?, wzr, #0xc
	mov x0, x3?
	ret
L4 (SSA Block: blk9):
L11 (SSA Block: blk4):
	movz w4?, #0xd, lsl 0
	mov x0, x4?
	ret
L5 (SSA Block: blk10):
L12 (SSA Block: blk3):
	orr w5?, wzr, #0xe
	mov x0, x5?
	ret
L6 (SSA Block: blk11):
L13 (SSA Block: blk2):
	orr w6?, wzr, #0xf
	mov x0, x6?
	ret
L7 (SSA Block: blk12):
L14 (SSA Block: blk1):
	orr w7?, wzr, #0x10
	mov x0, x7?
	ret
L8 (SSA Block: blk13):
L9 (SSA Block: blk6):
	movz w8?, #0xb, lsl 0
	mov x0, x8?
	ret
`,
			afterFinalizeARM64: `
L1 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	orr w8, wzr, #0x6
	subs wzr, w2, w8
	csel w8, w8, w2, hs
	adr x27, #16; ldrsw x8, [x27, x8, UXTW 2]; add x27, x27, x8; br x27; [0x1c 0x20 0x30 0x40 0x50 0x60 0x70]
L2 (SSA Block: blk7):
	b #0x54 (L9)
L3 (SSA Block: blk8):
L10 (SSA Block: blk5):
	orr w0, wzr, #0xc
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L4 (SSA Block: blk9):
L11 (SSA Block: blk4):
	movz w0, #0xd, lsl 0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L5 (SSA Block: blk10):
L12 (SSA Block: blk3):
	orr w0, wzr, #0xe
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L6 (SSA Block: blk11):
L13 (SSA Block: blk2):
	orr w0, wzr, #0xf
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L7 (SSA Block: blk12):
L14 (SSA Block: blk1):
	orr w0, wzr, #0x10
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L8 (SSA Block: blk13):
L9 (SSA Block: blk6):
	movz w0, #0xb, lsl 0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssab := ssa.NewBuilder()
			offset := wazevoapi.NewModuleContextOffsetData(tc.m)
			fc := frontend.NewFrontendCompiler(tc.m, ssab, &offset)
			machine := newMachine()
			machine.DisableStackCheck()
			be := backend.NewCompiler(context.Background(), machine, ssab)

			// Lowers the Wasm to SSA.
			typeIndex := tc.m.FunctionSection[tc.targetIndex]
			code := &tc.m.CodeSection[tc.targetIndex]
			fc.Init(tc.targetIndex, &tc.m.TypeSection[typeIndex], code.LocalTypes, code.Body)
			err := fc.LowerToSSA()
			require.NoError(t, err)
			if verbose {
				fmt.Println("============ SSA before passes ============")
				fmt.Println(ssab.Format())
			}

			// Need to run passes before lowering to machine code.
			ssab.RunPasses()
			if verbose {
				fmt.Println("============ SSA after passes ============")
				fmt.Println(ssab.Format())
			}

			ssab.LayoutBlocks()
			if verbose {
				fmt.Println("============ SSA after block layout ============")
				fmt.Println(ssab.Format())
			}

			// Lowers the SSA to ISA specific code.
			be.Lower()
			if verbose {
				fmt.Println("============ lowering result ============")
				fmt.Println(be.Format())
			}

			switch runtime.GOARCH {
			case "arm64":
				if tc.afterLoweringARM64 != "" {
					require.Equal(t, tc.afterLoweringARM64, be.Format())
				}
			default:
				t.Fail()
			}

			be.RegAlloc()
			if verbose {
				fmt.Println("============ regalloc result ============")
				fmt.Println(be.Format())
			}

			be.Finalize()
			if verbose {
				fmt.Println("============ finalization result ============")
				fmt.Println(be.Format())
			}

			switch runtime.GOARCH {
			case "arm64":
				require.Equal(t, tc.afterFinalizeARM64, be.Format())
			default:
				t.Fail()
			}

			// Sanity check on the final binary encoding.
			be.Encode()

			fmt.Println(hex.EncodeToString(be.Buf()))
		})
	}
}
