package backend_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/amd64"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/frontend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestMain(m *testing.M) {
	if !platform.CompilerSupports(api.CoreFeaturesV2 | experimental.CoreFeaturesThreads) {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func newMachine() backend.Machine {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.NewBackend()
	case "amd64":
		return amd64.NewBackend()
	default:
		panic("unsupported architecture")
	}
}

func TestE2E(t *testing.T) {
	const verbose = false

	type testCase struct {
		name                                   string
		m                                      *wasm.Module
		targetIndex                            uint32
		afterLoweringARM64, afterFinalizeARM64 string
		afterLoweringAMD64, afterFinalizeAMD64 string
	}

	for _, tc := range []testCase{
		{
			name: "empty", m: testcases.Empty.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	ldr d133?, #8; b 16; data.f64 64.000000
	mov v1.8b, v133?.8b
	ldr s132?, #8; b 8; data.f32 32.000000
	mov v0.8b, v132?.8b
	orr x131?, xzr, #0x2
	mov x1, x131?
	orr w130?, wzr, #0x1
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
	add w132?, w130?, w131?
	sub w133?, w132?, w130?
	mov x0, x133?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov v131?.8b, v0.8b
	mov v132?.8b, v1.8b
	add x133?, x130?, x130?
	sub x134?, x133?, x130?
	fadd s135?, s131?, s131?
	fsub s136?, s135?, s131?
	fmul s137?, s136?, s131?
	fdiv s138?, s137?, s131?
	fmax s139?, s138?, s131?
	fmin s140?, s139?, s131?
	fadd d141?, d132?, d132?
	fsub d142?, d141?, d132?
	fmul d143?, d142?, d132?
	fdiv d144?, d143?, d132?
	fmax d145?, d144?, d132?
	fmin d146?, d145?, d132?
	mov v1.8b, v146?.8b
	mov v0.8b, v140?.8b
	mov x0, x134?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, xzr
	mov x1, x131?
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
	mov x1, x130?
	mov x0, x131?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
L1 (SSA Block: blk1):
	mov x1, x130?
	mov x0, x131?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
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
L0 (SSA Block: blk0):
L1 (SSA Block: blk1):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "block_br_if", m: testcases.BlockBrIf.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x131?, xzr
	cbnz w131?, L1
L2 (SSA Block: blk2):
	movz x132?, #0x3, lsl 0
	str w132?, [x128?]
	mov x133?, sp
	str x133?, [x128?, #0x38]
	adr x134?, #0x0
	str x134?, [x128?, #0x30]
	exit_sequence x128?
L1 (SSA Block: blk1):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbnz w8, #0x34 (L1)
L2 (SSA Block: blk2):
	movz x8, #0x3, lsl 0
	str w8, [x0]
	mov x8, sp
	str x8, [x0, #0x38]
	adr x8, #0x0
	str x8, [x0, #0x30]
	exit_sequence x0
L1 (SSA Block: blk1):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "loop_br", m: testcases.LoopBr.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
L1 (SSA Block: blk1):
	b L1
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	b #0x0 (L1)
`,
		},
		{
			name: "loop_with_param_results", m: testcases.LoopBrWithParamResults.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
L1 (SSA Block: blk1):
	orr w133?, wzr, #0x1
	cbz w133?, (L3)
L4 (SSA Block: blk4):
	b L1
L3 (SSA Block: blk3):
L2 (SSA Block: blk2):
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	orr w8, wzr, #0x1
	cbz w8, #0x8 L3
L4 (SSA Block: blk4):
	b #-0x8 (L1)
L3 (SSA Block: blk3):
L2 (SSA Block: blk2):
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "loop_br_if", m: testcases.LoopBrIf.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
L1 (SSA Block: blk1):
	orr w131?, wzr, #0x1
	cbz w131?, (L3)
L4 (SSA Block: blk4):
	b L1
L3 (SSA Block: blk3):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	orr w8, wzr, #0x1
	cbz w8, #0x8 L3
L4 (SSA Block: blk4):
	b #-0x8 (L1)
L3 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "block_block_br", m: testcases.BlockBlockBr.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
L1 (SSA Block: blk1):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
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
L0 (SSA Block: blk0):
	mov x131?, xzr
	cbz w131?, (L2)
L1 (SSA Block: blk1):
	b L3
L2 (SSA Block: blk2):
L3 (SSA Block: blk3):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbz w8, #0x8 L2
L1 (SSA Block: blk1):
	b #0x4 (L3)
L2 (SSA Block: blk2):
L3 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "if_else", m: testcases.IfElse.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x131?, xzr
	cbz w131?, (L2)
L1 (SSA Block: blk1):
L3 (SSA Block: blk3):
	ret
L2 (SSA Block: blk2):
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbz w8, #0x10 L2
L1 (SSA Block: blk1):
L3 (SSA Block: blk3):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk2):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "single_predecessor_local_refs", m: testcases.SinglePredecessorLocalRefs.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x131?, xzr
	cbz w131?, (L2)
L1 (SSA Block: blk1):
	mov x130?, xzr
	mov x0, x130?
	ret
L2 (SSA Block: blk2):
L3 (SSA Block: blk3):
	mov x130?, xzr
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	cbz w8, #0x14 L2
L1 (SSA Block: blk1):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk2):
L3 (SSA Block: blk3):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "multi_predecessor_local_ref", m: testcases.MultiPredecessorLocalRef.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
	cbz w130?, (L2)
L1 (SSA Block: blk1):
	mov x132?, x130?
	b L3
L2 (SSA Block: blk2):
	mov x132?, x131?
L3 (SSA Block: blk3):
	mov x0, x132?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	cbz w2, #0x8 L2
L1 (SSA Block: blk1):
	b #0x8 (L3)
L2 (SSA Block: blk2):
	mov x2, x3
L3 (SSA Block: blk3):
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "reference_value_from_unsealed_block", m: testcases.ReferenceValueFromUnsealedBlock.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
L1 (SSA Block: blk1):
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	mov x0, x2
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "reference_value_from_unsealed_block2", m: testcases.ReferenceValueFromUnsealedBlock2.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
L1 (SSA Block: blk1):
	cbz w130?, (L4)
L5 (SSA Block: blk5):
	b L1
L4 (SSA Block: blk4):
L3 (SSA Block: blk3):
L2 (SSA Block: blk2):
	mov x131?, xzr
	mov x0, x131?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	cbz w2, #0x8 L4
L5 (SSA Block: blk5):
	b #-0x4 (L1)
L4 (SSA Block: blk4):
L3 (SSA Block: blk3):
L2 (SSA Block: blk2):
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
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x130?
L1 (SSA Block: blk1):
	cbnz w131?, L5
	b L4
L5 (SSA Block: blk5):
	ret
L4 (SSA Block: blk4):
L3 (SSA Block: blk3):
	load_const_block_arg x131?, 0x1
	b L1
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
L1 (SSA Block: blk1):
	cbnz w2, #0x8 (L5)
	b #0x10 (L4)
L5 (SSA Block: blk5):
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L4 (SSA Block: blk4):
L3 (SSA Block: blk3):
	load_const_block_arg x2, 0x1
	orr w2, wzr, #0x1
	b #-0x18 (L1)
`,
		},
		{
			name: "call", m: testcases.Call.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	mov x0, x128?
	mov x1, x129?
	bl f1
	mov x130?, x0
	mov x0, x128?
	mov x1, x129?
	mov x2, x130?
	movz w131?, #0x5, lsl 0
	mov x3, x131?
	bl f2
	mov x132?, x0
	mov x0, x128?
	mov x1, x129?
	mov x2, x132?
	bl f3
	mov x133?, x0
	mov x134?, x1
	mov x1, x134?
	mov x0, x133?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x10
	orr x27, xzr, #0x10
	str x27, [sp, #-0x10]!
	str x0, [sp, #0x10]
	str x1, [sp, #0x18]
	bl f1
	mov x2, x0
	ldr x8, [sp, #0x10]
	mov x0, x8
	ldr x9, [sp, #0x18]
	mov x1, x9
	movz w3, #0x5, lsl 0
	bl f2
	mov x2, x0
	ldr x8, [sp, #0x10]
	mov x0, x8
	ldr x8, [sp, #0x18]
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
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	mov x130?, x2
	mov x131?, x3
	mov v132?.8b, v0.8b
	mov v133?.8b, v1.8b
	mov x0, x128?
	mov x1, x129?
	mov x2, x130?
	mov x3, x131?
	mov v0.8b, v132?.8b
	mov v1.8b, v133?.8b
	mov x4, x130?
	mov x5, x131?
	mov v2.8b, v132?.8b
	mov v3.8b, v133?.8b
	mov x6, x130?
	mov x7, x131?
	mov v4.8b, v132?.8b
	mov v5.8b, v133?.8b
	str w130?, [sp, #-0xd0]
	str x131?, [sp, #-0xc8]
	mov v6.8b, v132?.8b
	mov v7.8b, v133?.8b
	str w130?, [sp, #-0xc0]
	str x131?, [sp, #-0xb8]
	str s132?, [sp, #-0xb0]
	str d133?, [sp, #-0xa8]
	str w130?, [sp, #-0xa0]
	str x131?, [sp, #-0x98]
	str s132?, [sp, #-0x90]
	str d133?, [sp, #-0x88]
	str w130?, [sp, #-0x80]
	str x131?, [sp, #-0x78]
	str s132?, [sp, #-0x70]
	str d133?, [sp, #-0x68]
	str w130?, [sp, #-0x60]
	str x131?, [sp, #-0x58]
	str s132?, [sp, #-0x50]
	str d133?, [sp, #-0x48]
	str w130?, [sp, #-0x40]
	str x131?, [sp, #-0x38]
	str s132?, [sp, #-0x30]
	str d133?, [sp, #-0x28]
	str w130?, [sp, #-0x20]
	str x131?, [sp, #-0x18]
	str s132?, [sp, #-0x10]
	str d133?, [sp, #-0x8]
	bl f1
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x20
	orr x27, xzr, #0x20
	str x27, [sp, #-0x10]!
	str w2, [sp, #0x10]
	str x3, [sp, #0x14]
	str s0, [sp, #0x1c]
	str d1, [sp, #0x20]
	ldr w8, [sp, #0x10]
	mov x4, x8
	ldr x9, [sp, #0x14]
	mov x5, x9
	ldr s8, [sp, #0x1c]
	mov v2.8b, v8.8b
	ldr d9, [sp, #0x20]
	mov v3.8b, v9.8b
	mov x6, x8
	mov x7, x9
	mov v4.8b, v8.8b
	mov v5.8b, v9.8b
	str w8, [sp, #-0xd0]
	str x9, [sp, #-0xc8]
	mov v6.8b, v8.8b
	mov v7.8b, v9.8b
	str w8, [sp, #-0xc0]
	str x9, [sp, #-0xb8]
	str s8, [sp, #-0xb0]
	str d9, [sp, #-0xa8]
	str w8, [sp, #-0xa0]
	str x9, [sp, #-0x98]
	str s8, [sp, #-0x90]
	str d9, [sp, #-0x88]
	str w8, [sp, #-0x80]
	str x9, [sp, #-0x78]
	str s8, [sp, #-0x70]
	str d9, [sp, #-0x68]
	str w8, [sp, #-0x60]
	str x9, [sp, #-0x58]
	str s8, [sp, #-0x50]
	str d9, [sp, #-0x48]
	str w8, [sp, #-0x40]
	str x9, [sp, #-0x38]
	str s8, [sp, #-0x30]
	str d9, [sp, #-0x28]
	str w8, [sp, #-0x20]
	str x9, [sp, #-0x18]
	str s8, [sp, #-0x10]
	str d9, [sp, #-0x8]
	bl f1
	add sp, sp, #0x10
	add sp, sp, #0x20
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "call_many_returns", m: testcases.CallManyReturns.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	mov x130?, x2
	mov x131?, x3
	mov v132?.8b, v0.8b
	mov v133?.8b, v1.8b
	mov x0, x128?
	mov x1, x129?
	mov x2, x130?
	mov x3, x131?
	mov v0.8b, v132?.8b
	mov v1.8b, v133?.8b
	bl f1
	mov x134?, x0
	mov x135?, x1
	mov v136?.8b, v0.8b
	mov v137?.8b, v1.8b
	mov x138?, x2
	mov x139?, x3
	mov v140?.8b, v2.8b
	mov v141?.8b, v3.8b
	mov x142?, x4
	mov x143?, x5
	mov v144?.8b, v4.8b
	mov v145?.8b, v5.8b
	mov x146?, x6
	mov x147?, x7
	mov v148?.8b, v6.8b
	mov v149?.8b, v7.8b
	ldr w150?, [sp, #-0xc0]
	ldr x151?, [sp, #-0xb8]
	ldr s152?, [sp, #-0xb0]
	ldr d153?, [sp, #-0xa8]
	ldr w154?, [sp, #-0xa0]
	ldr x155?, [sp, #-0x98]
	ldr s156?, [sp, #-0x90]
	ldr d157?, [sp, #-0x88]
	ldr w158?, [sp, #-0x80]
	ldr x159?, [sp, #-0x78]
	ldr s160?, [sp, #-0x70]
	ldr d161?, [sp, #-0x68]
	ldr w162?, [sp, #-0x60]
	ldr x163?, [sp, #-0x58]
	ldr s164?, [sp, #-0x50]
	ldr d165?, [sp, #-0x48]
	ldr w166?, [sp, #-0x40]
	ldr x167?, [sp, #-0x38]
	ldr s168?, [sp, #-0x30]
	ldr d169?, [sp, #-0x28]
	ldr w170?, [sp, #-0x20]
	ldr x171?, [sp, #-0x18]
	ldr s172?, [sp, #-0x10]
	ldr d173?, [sp, #-0x8]
	str d173?, [#ret_space, #0xb8]
	str s172?, [#ret_space, #0xb0]
	str x171?, [#ret_space, #0xa8]
	str w170?, [#ret_space, #0xa0]
	str d169?, [#ret_space, #0x98]
	str s168?, [#ret_space, #0x90]
	str x167?, [#ret_space, #0x88]
	str w166?, [#ret_space, #0x80]
	str d165?, [#ret_space, #0x78]
	str s164?, [#ret_space, #0x70]
	str x163?, [#ret_space, #0x68]
	str w162?, [#ret_space, #0x60]
	str d161?, [#ret_space, #0x58]
	str s160?, [#ret_space, #0x50]
	str x159?, [#ret_space, #0x48]
	str w158?, [#ret_space, #0x40]
	str d157?, [#ret_space, #0x38]
	str s156?, [#ret_space, #0x30]
	str x155?, [#ret_space, #0x28]
	str w154?, [#ret_space, #0x20]
	str d153?, [#ret_space, #0x18]
	str s152?, [#ret_space, #0x10]
	str x151?, [#ret_space, #0x8]
	str w150?, [#ret_space, #0x0]
	mov v7.8b, v149?.8b
	mov v6.8b, v148?.8b
	mov x7, x147?
	mov x6, x146?
	mov v5.8b, v145?.8b
	mov v4.8b, v144?.8b
	mov x5, x143?
	mov x4, x142?
	mov v3.8b, v141?.8b
	mov v2.8b, v140?.8b
	mov x3, x139?
	mov x2, x138?
	mov v1.8b, v137?.8b
	mov v0.8b, v136?.8b
	mov x1, x135?
	mov x0, x134?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	orr x27, xzr, #0xc0
	sub sp, sp, x27
	stp x30, x27, [sp, #-0x10]!
	str x19, [sp, #-0x10]!
	str x20, [sp, #-0x10]!
	str q18, [sp, #-0x10]!
	str q19, [sp, #-0x10]!
	orr x27, xzr, #0x40
	str x27, [sp, #-0x10]!
	bl f1
	ldr w8, [sp, #-0xc0]
	ldr x9, [sp, #-0xb8]
	ldr s8, [sp, #-0xb0]
	ldr d9, [sp, #-0xa8]
	ldr w10, [sp, #-0xa0]
	ldr x11, [sp, #-0x98]
	ldr s10, [sp, #-0x90]
	ldr d11, [sp, #-0x88]
	ldr w12, [sp, #-0x80]
	ldr x13, [sp, #-0x78]
	ldr s12, [sp, #-0x70]
	ldr d13, [sp, #-0x68]
	ldr w14, [sp, #-0x60]
	ldr x15, [sp, #-0x58]
	ldr s14, [sp, #-0x50]
	ldr d15, [sp, #-0x48]
	ldr w16, [sp, #-0x40]
	ldr x17, [sp, #-0x38]
	ldr s16, [sp, #-0x30]
	ldr d17, [sp, #-0x28]
	ldr w19, [sp, #-0x20]
	ldr x20, [sp, #-0x18]
	ldr s18, [sp, #-0x10]
	ldr d19, [sp, #-0x8]
	str d19, [sp, #0x118]
	str s18, [sp, #0x110]
	str x20, [sp, #0x108]
	str w19, [sp, #0x100]
	str d17, [sp, #0xf8]
	str s16, [sp, #0xf0]
	str x17, [sp, #0xe8]
	str w16, [sp, #0xe0]
	str d15, [sp, #0xd8]
	str s14, [sp, #0xd0]
	str x15, [sp, #0xc8]
	str w14, [sp, #0xc0]
	str d13, [sp, #0xb8]
	str s12, [sp, #0xb0]
	str x13, [sp, #0xa8]
	str w12, [sp, #0xa0]
	str d11, [sp, #0x98]
	str s10, [sp, #0x90]
	str x11, [sp, #0x88]
	str w10, [sp, #0x80]
	str d9, [sp, #0x78]
	str s8, [sp, #0x70]
	str x9, [sp, #0x68]
	str w8, [sp, #0x60]
	add sp, sp, #0x10
	ldr q19, [sp], #0x10
	ldr q18, [sp], #0x10
	ldr x20, [sp], #0x10
	ldr x19, [sp], #0x10
	ldr x30, [sp], #0x10
	add sp, sp, #0xc0
	ret
`,
		},
		{
			name: "integer_extensions",
			m:    testcases.IntegerExtensions.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
	sxtw x132?, w130?
	uxtw x133?, w130?
	sxtb x134?, w131?
	sxth x135?, w131?
	sxtw x136?, w131?
	sxtb w137?, w130?
	sxth w138?, w130?
	mov x6, x138?
	mov x5, x137?
	mov x4, x136?
	mov x3, x135?
	mov x2, x134?
	mov x1, x133?
	mov x0, x132?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	sxtw x0, w2
	uxtw x1, w2
	sxtb x8, w3
	sxth x9, w3
	sxtw x4, w3
	sxtb w5, w2
	sxth w6, w2
	mov x3, x9
	mov x2, x8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "integer bit counts", m: testcases.IntegerBitCounts.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
	mov x131?, x3
	clz w132?, w130?
	rbit w145?, w130?
	clz w133?, w145?
	ins v142?.d[0], x130?
	cnt v143?.16b, v142?.16b
	uaddlv h144?, v143?.8b
	mov x134?, v144?.d[0]
	clz x135?, x131?
	rbit x141?, x131?
	clz x136?, x141?
	ins v138?.d[0], x131?
	cnt v139?.16b, v138?.16b
	uaddlv h140?, v139?.8b
	mov x137?, v140?.d[0]
	mov x5, x137?
	mov x4, x136?
	mov x3, x135?
	mov x2, x134?
	mov x1, x133?
	mov x0, x132?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	clz w0, w2
	rbit w8, w2
	clz w1, w8
	ins v8.d[0], x2
	cnt v8.16b, v8.16b
	uaddlv h8, v8.8b
	mov x2, v8.d[0]
	clz x8, x3
	rbit x9, x3
	clz x4, x9
	ins v8.d[0], x3
	cnt v8.16b, v8.16b
	uaddlv h8, v8.8b
	mov x5, v8.d[0]
	mov x3, x8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "float_comparisons",
			m:    testcases.FloatComparisons.Module,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	orr x27, xzr, #0x20
	sub sp, sp, x27
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
	cset x8, mi
	fcmp d2, d3
	cset x9, gt
	fcmp d2, d3
	cset x10, ls
	fcmp d2, d3
	cset x11, ge
	str w11, [sp, #0x38]
	str w10, [sp, #0x30]
	str w9, [sp, #0x28]
	str w8, [sp, #0x20]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	add sp, sp, #0x20
	ret
`,
		},
		{
			name: "float_conversions",
			m:    testcases.FloatConversions.Module,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	msr fpsr, xzr
	fcvtzs x8, d0
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v0.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L17)
	fcmp d8, d8
	mov x9, x10
	b.vc #0x34, (L16)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L16:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L17:
	msr fpsr, xzr
	fcvtzs x1, s1
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v1.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L15)
	fcmp s8, s8
	mov x9, x10
	b.vc #0x34, (L14)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L14:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L15:
	msr fpsr, xzr
	fcvtzs w2, d0
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v0.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L13)
	fcmp d8, d8
	mov x9, x10
	b.vc #0x34, (L12)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L12:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L13:
	msr fpsr, xzr
	fcvtzs w3, s1
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v1.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L11)
	fcmp s8, s8
	mov x9, x10
	b.vc #0x34, (L10)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L10:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L11:
	msr fpsr, xzr
	fcvtzu x4, d0
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v0.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L9)
	fcmp d8, d8
	mov x9, x10
	b.vc #0x34, (L8)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L8:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L9:
	msr fpsr, xzr
	fcvtzu x5, s1
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v1.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L7)
	fcmp s8, s8
	mov x9, x10
	b.vc #0x34, (L6)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L6:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L7:
	msr fpsr, xzr
	fcvtzu w6, d0
	mrs x9 fpsr
	mov x10, x0
	mov v8.16b, v0.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L5)
	fcmp d8, d8
	mov x9, x10
	b.vc #0x34, (L4)
	movz x11, #0xc, lsl 0
	str w11, [x9]
	mov x11, sp
	str x11, [x9, #0x38]
	adr x11, #0x0
	str x11, [x9, #0x30]
	exit_sequence x9
L4:
	movz x9, #0xb, lsl 0
	str w9, [x10]
	mov x9, sp
	str x9, [x10, #0x38]
	adr x9, #0x0
	str x9, [x10, #0x30]
	exit_sequence x10
L5:
	msr fpsr, xzr
	fcvtzu w7, s1
	mrs x9 fpsr
	mov v8.16b, v1.16b
	subs xzr, x9, #0x1
	b.ne #0x70, (L3)
	fcmp s8, s8
	mov x9, x0
	b.vc #0x34, (L2)
	movz x10, #0xc, lsl 0
	str w10, [x9]
	mov x10, sp
	str x10, [x9, #0x38]
	adr x10, #0x0
	str x10, [x9, #0x30]
	exit_sequence x9
L2:
	movz x9, #0xb, lsl 0
	str w9, [x0]
	mov x9, sp
	str x9, [x0, #0x38]
	adr x9, #0x0
	str x9, [x0, #0x30]
	exit_sequence x0
L3:
	fcvt s0, d0
	fcvt d1, s1
	mov x0, x8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "nontrapping_float_conversions",
			m:    testcases.NonTrappingFloatConversions.Module,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	fcvtzs x8, d0
	fcvtzs x1, s1
	fcvtzs w2, d0
	fcvtzs w3, s1
	fcvtzu x4, d0
	fcvtzu x5, s1
	fcvtzu w6, d0
	fcvtzu w7, s1
	mov x0, x8
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "many_middle_values",
			m:    testcases.ManyMiddleValues.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x130?, x2
	mov v131?.8b, v0.8b
	orr w289?, wzr, #0x1
	madd w133?, w130?, w289?, wzr
	orr w288?, wzr, #0x2
	madd w135?, w130?, w288?, wzr
	orr w287?, wzr, #0x3
	madd w137?, w130?, w287?, wzr
	orr w286?, wzr, #0x4
	madd w139?, w130?, w286?, wzr
	movz w285?, #0x5, lsl 0
	madd w141?, w130?, w285?, wzr
	orr w284?, wzr, #0x6
	madd w143?, w130?, w284?, wzr
	orr w283?, wzr, #0x7
	madd w145?, w130?, w283?, wzr
	orr w282?, wzr, #0x8
	madd w147?, w130?, w282?, wzr
	movz w281?, #0x9, lsl 0
	madd w149?, w130?, w281?, wzr
	movz w280?, #0xa, lsl 0
	madd w151?, w130?, w280?, wzr
	movz w279?, #0xb, lsl 0
	madd w153?, w130?, w279?, wzr
	orr w278?, wzr, #0xc
	madd w155?, w130?, w278?, wzr
	movz w277?, #0xd, lsl 0
	madd w157?, w130?, w277?, wzr
	orr w276?, wzr, #0xe
	madd w159?, w130?, w276?, wzr
	orr w275?, wzr, #0xf
	madd w161?, w130?, w275?, wzr
	orr w274?, wzr, #0x10
	madd w163?, w130?, w274?, wzr
	movz w273?, #0x11, lsl 0
	madd w165?, w130?, w273?, wzr
	movz w272?, #0x12, lsl 0
	madd w167?, w130?, w272?, wzr
	movz w271?, #0x13, lsl 0
	madd w169?, w130?, w271?, wzr
	movz w270?, #0x14, lsl 0
	madd w171?, w130?, w270?, wzr
	add w172?, w169?, w171?
	add w173?, w167?, w172?
	add w174?, w165?, w173?
	add w175?, w163?, w174?
	add w176?, w161?, w175?
	add w177?, w159?, w176?
	add w178?, w157?, w177?
	add w179?, w155?, w178?
	add w180?, w153?, w179?
	add w181?, w151?, w180?
	add w182?, w149?, w181?
	add w183?, w147?, w182?
	add w184?, w145?, w183?
	add w185?, w143?, w184?
	add w186?, w141?, w185?
	add w187?, w139?, w186?
	add w188?, w137?, w187?
	add w189?, w135?, w188?
	add w190?, w133?, w189?
	ldr s269?, #8; b 8; data.f32 1.000000
	fmul s192?, s131?, s269?
	ldr s268?, #8; b 8; data.f32 2.000000
	fmul s194?, s131?, s268?
	ldr s267?, #8; b 8; data.f32 3.000000
	fmul s196?, s131?, s267?
	ldr s266?, #8; b 8; data.f32 4.000000
	fmul s198?, s131?, s266?
	ldr s265?, #8; b 8; data.f32 5.000000
	fmul s200?, s131?, s265?
	ldr s264?, #8; b 8; data.f32 6.000000
	fmul s202?, s131?, s264?
	ldr s263?, #8; b 8; data.f32 7.000000
	fmul s204?, s131?, s263?
	ldr s262?, #8; b 8; data.f32 8.000000
	fmul s206?, s131?, s262?
	ldr s261?, #8; b 8; data.f32 9.000000
	fmul s208?, s131?, s261?
	ldr s260?, #8; b 8; data.f32 10.000000
	fmul s210?, s131?, s260?
	ldr s259?, #8; b 8; data.f32 11.000000
	fmul s212?, s131?, s259?
	ldr s258?, #8; b 8; data.f32 12.000000
	fmul s214?, s131?, s258?
	ldr s257?, #8; b 8; data.f32 13.000000
	fmul s216?, s131?, s257?
	ldr s256?, #8; b 8; data.f32 14.000000
	fmul s218?, s131?, s256?
	ldr s255?, #8; b 8; data.f32 15.000000
	fmul s220?, s131?, s255?
	ldr s254?, #8; b 8; data.f32 16.000000
	fmul s222?, s131?, s254?
	ldr s253?, #8; b 8; data.f32 17.000000
	fmul s224?, s131?, s253?
	ldr s252?, #8; b 8; data.f32 18.000000
	fmul s226?, s131?, s252?
	ldr s251?, #8; b 8; data.f32 19.000000
	fmul s228?, s131?, s251?
	ldr s250?, #8; b 8; data.f32 20.000000
	fmul s230?, s131?, s250?
	fadd s231?, s228?, s230?
	fadd s232?, s226?, s231?
	fadd s233?, s224?, s232?
	fadd s234?, s222?, s233?
	fadd s235?, s220?, s234?
	fadd s236?, s218?, s235?
	fadd s237?, s216?, s236?
	fadd s238?, s214?, s237?
	fadd s239?, s212?, s238?
	fadd s240?, s210?, s239?
	fadd s241?, s208?, s240?
	fadd s242?, s206?, s241?
	fadd s243?, s204?, s242?
	fadd s244?, s202?, s243?
	fadd s245?, s200?, s244?
	fadd s246?, s198?, s245?
	fadd s247?, s196?, s246?
	fadd s248?, s194?, s247?
	fadd s249?, s192?, s248?
	mov v0.8b, v249?.8b
	mov x0, x190?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
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
	movz w19, #0xb, lsl 0
	madd w19, w2, w19, wzr
	orr w20, wzr, #0xc
	madd w20, w2, w20, wzr
	movz w21, #0xd, lsl 0
	madd w21, w2, w21, wzr
	orr w22, wzr, #0xe
	madd w22, w2, w22, wzr
	orr w23, wzr, #0xf
	madd w23, w2, w23, wzr
	orr w24, wzr, #0x10
	madd w24, w2, w24, wzr
	movz w25, #0x11, lsl 0
	madd w25, w2, w25, wzr
	movz w26, #0x12, lsl 0
	madd w26, w2, w26, wzr
	movz w29, #0x13, lsl 0
	madd w29, w2, w29, wzr
	movz w30, #0x14, lsl 0
	madd w30, w2, w30, wzr
	add w29, w29, w30
	add w26, w26, w29
	add w25, w25, w26
	add w24, w24, w25
	add w23, w23, w24
	add w22, w22, w23
	add w21, w21, w22
	add w20, w20, w21
	add w19, w19, w20
	add w17, w17, w19
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
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	mov x130?, x2
	str x129?, [x128?, #0x8]
	ldr x131?, [x129?, #0x8]
	ldr x132?, [x129?, #0x10]
	mov x0, x128?
	mov x1, x132?
	mov x2, x130?
	mov x3, x130?
	bl x131?
	mov x133?, x0
	mov x0, x133?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x10
	orr x27, xzr, #0x10
	str x27, [sp, #-0x10]!
	str w2, [sp, #0x10]
	str x1, [x0, #0x8]
	ldr x8, [x1, #0x8]
	ldr x1, [x1, #0x10]
	ldr w9, [sp, #0x10]
	mov x3, x9
	bl x8
	add sp, sp, #0x10
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "memory_load_basic", m: testcases.MemoryLoadBasic.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	mov x130?, x2
	uxtw x132?, w130?
	ldr w133?, [x129?, #0x10]
	add x134?, x132?, #0x4
	subs xzr, x133?, x134?
	mov x140?, x128?
	b.hs L2
	movz x141?, #0x4, lsl 0
	str w141?, [x140?]
	mov x142?, sp
	str x142?, [x140?, #0x38]
	adr x143?, #0x0
	str x143?, [x140?, #0x30]
	exit_sequence x140?
L2:
	ldr x136?, [x129?, #0x8]
	add x139?, x136?, x132?
	ldr w138?, [x139?]
	mov x0, x138?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	uxtw x8, w2
	ldr w9, [x1, #0x10]
	add x10, x8, #0x4
	subs xzr, x9, x10
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
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	mov x8, xzr
	uxtw x8, w8
	ldr w9, [x1, #0x10]
	add x10, x8, #0x4
	subs xzr, x9, x10
	mov x10, x0
	b.hs #0x34, (L10)
	movz x11, #0x4, lsl 0
	str w11, [x10]
	mov x11, sp
	str x11, [x10, #0x38]
	adr x11, #0x0
	str x11, [x10, #0x30]
	exit_sequence x10
L10:
	ldr x10, [x1, #0x8]
	add x8, x10, x8
	str w2, [x8]
	orr w8, wzr, #0x8
	uxtw x8, w8
	add x11, x8, #0x8
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L9)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L9:
	add x8, x10, x8
	str x3, [x8]
	orr w8, wzr, #0x10
	uxtw x8, w8
	add x11, x8, #0x4
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L8)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L8:
	add x8, x10, x8
	str s0, [x8]
	orr w8, wzr, #0x18
	uxtw x8, w8
	add x11, x8, #0x8
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L7)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L7:
	add x8, x10, x8
	str d1, [x8]
	orr w8, wzr, #0x20
	uxtw x8, w8
	add x11, x8, #0x1
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L6)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L6:
	add x8, x10, x8
	strb w2, [x8]
	movz w8, #0x28, lsl 0
	uxtw x8, w8
	add x11, x8, #0x2
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L5)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L5:
	add x8, x10, x8
	strh w2, [x8]
	orr w8, wzr, #0x30
	uxtw x8, w8
	add x11, x8, #0x1
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L4)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L4:
	add x8, x10, x8
	strb w3, [x8]
	orr w8, wzr, #0x38
	uxtw x8, w8
	add x11, x8, #0x2
	subs xzr, x9, x11
	mov x11, x0
	b.hs #0x34, (L3)
	movz x12, #0x4, lsl 0
	str w12, [x11]
	mov x12, sp
	str x12, [x11, #0x38]
	adr x12, #0x0
	str x12, [x11, #0x30]
	exit_sequence x11
L3:
	add x8, x10, x8
	strh w3, [x8]
	orr w8, wzr, #0x40
	uxtw x8, w8
	add x11, x8, #0x4
	subs xzr, x9, x11
	b.hs #0x34, (L2)
	movz x9, #0x4, lsl 0
	str w9, [x0]
	mov x9, sp
	str x9, [x0, #0x38]
	adr x9, #0x0
	str x9, [x0, #0x30]
	exit_sequence x0
L2:
	add x8, x10, x8
	str w3, [x8]
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "globals_mutable[0]",
			m:    testcases.GlobalsMutable.Module,
			afterLoweringARM64: `
L0 (SSA Block: blk0):
	mov x128?, x0
	mov x129?, x1
	ldr w130?, [x129?, #0x10]
	ldr x131?, [x129?, #0x20]
	ldr s132?, [x129?, #0x30]
	ldr d133?, [x129?, #0x40]
	mov x0, x128?
	mov x1, x129?
	bl f1
	ldr w134?, [x129?, #0x10]
	ldr x135?, [x129?, #0x20]
	ldr s136?, [x129?, #0x30]
	ldr d137?, [x129?, #0x40]
	mov v3.8b, v137?.8b
	mov v2.8b, v136?.8b
	mov x3, x135?
	mov x2, x134?
	mov v1.8b, v133?.8b
	mov v0.8b, v132?.8b
	mov x1, x131?
	mov x0, x130?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	sub sp, sp, #0x20
	orr x27, xzr, #0x20
	str x27, [sp, #-0x10]!
	str x1, [sp, #0x10]
	ldr w8, [x1, #0x10]
	str w8, [sp, #0x2c]
	ldr x9, [x1, #0x20]
	str x9, [sp, #0x24]
	ldr s0, [x1, #0x30]
	str s0, [sp, #0x20]
	ldr d1, [x1, #0x40]
	str d1, [sp, #0x18]
	bl f1
	ldr x8, [sp, #0x10]
	ldr w2, [x8, #0x10]
	ldr x3, [x8, #0x20]
	ldr s2, [x8, #0x30]
	ldr d3, [x8, #0x40]
	ldr d1, [sp, #0x18]
	ldr s0, [sp, #0x20]
	ldr x1, [sp, #0x24]
	ldr w0, [sp, #0x2c]
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
L0 (SSA Block: blk0):
	mov x129?, x1
	orr w137?, wzr, #0x1
	str w137?, [x129?, #0x10]
	orr x136?, xzr, #0x2
	str x136?, [x129?, #0x20]
	ldr s135?, #8; b 8; data.f32 3.000000
	str s135?, [x129?, #0x30]
	ldr d134?, #8; b 16; data.f64 4.000000
	str d134?, [x129?, #0x40]
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	orr w8, wzr, #0x1
	str w8, [x1, #0x10]
	orr x8, xzr, #0x2
	str x8, [x1, #0x20]
	ldr s8, #8; b 8; data.f32 3.000000
	str s8, [x1, #0x30]
	ldr d8, #8; b 16; data.f64 4.000000
	str d8, [x1, #0x40]
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
L0 (SSA Block: blk0):
	mov x130?, x2
	orr w137?, wzr, #0x6
	subs wzr, w130?, w137?
	csel w138?, w137?, w130?, hs
	br_table_sequence x138?, table_index=0
L7 (SSA Block: blk7):
	b L6
L8 (SSA Block: blk8):
L5 (SSA Block: blk5):
	orr w131?, wzr, #0xc
	mov x0, x131?
	ret
L9 (SSA Block: blk9):
L4 (SSA Block: blk4):
	movz w132?, #0xd, lsl 0
	mov x0, x132?
	ret
L10 (SSA Block: blk10):
L3 (SSA Block: blk3):
	orr w133?, wzr, #0xe
	mov x0, x133?
	ret
L11 (SSA Block: blk11):
L2 (SSA Block: blk2):
	orr w134?, wzr, #0xf
	mov x0, x134?
	ret
L12 (SSA Block: blk12):
L1 (SSA Block: blk1):
	orr w135?, wzr, #0x10
	mov x0, x135?
	ret
L13 (SSA Block: blk13):
L6 (SSA Block: blk6):
	movz w136?, #0xb, lsl 0
	mov x0, x136?
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	orr w8, wzr, #0x6
	subs wzr, w2, w8
	csel w8, w8, w2, hs
	br_table_sequence x8, table_index=0
L7 (SSA Block: blk7):
	b #0x54 (L6)
L8 (SSA Block: blk8):
L5 (SSA Block: blk5):
	orr w0, wzr, #0xc
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L9 (SSA Block: blk9):
L4 (SSA Block: blk4):
	movz w0, #0xd, lsl 0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L10 (SSA Block: blk10):
L3 (SSA Block: blk3):
	orr w0, wzr, #0xe
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L11 (SSA Block: blk11):
L2 (SSA Block: blk2):
	orr w0, wzr, #0xf
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L12 (SSA Block: blk12):
L1 (SSA Block: blk1):
	orr w0, wzr, #0x10
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L13 (SSA Block: blk13):
L6 (SSA Block: blk6):
	movz w0, #0xb, lsl 0
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "VecShuffle",
			m:    testcases.VecShuffle.Module,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str q29, [sp, #-0x10]!
	str q30, [sp, #-0x10]!
	orr x27, xzr, #0x20
	str x27, [sp, #-0x10]!
	mov v29.16b, v0.16b
	mov v30.16b, v1.16b
	ldr q8, #8; b 32; data.v128  0706050403020100 1f1e1d1c1b1a1918
	tbl v0.16b, { v29.16b, v30.16b }, v8.16b
	add sp, sp, #0x10
	ldr q30, [sp], #0x10
	ldr q29, [sp], #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
		{
			name: "AtomicRmwAdd",
			m:    testcases.AtomicRmwAdd.Module,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	orr x27, xzr, #0x10
	sub sp, sp, x27
	stp x30, x27, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	ldr x8, [sp, #0x20]
	mov x9, xzr
	uxtw x9, w9
	add x10, x1, #0x10
	ldar x10, x10
	add x11, x9, #0x1
	subs xzr, x10, x11
	mov x10, x0
	b.hs #0x34, (L13)
	movz x11, #0x4, lsl 0
	str w11, [x10]
	mov x11, sp
	str x11, [x10, #0x38]
	adr x11, #0x0
	str x11, [x10, #0x30]
	exit_sequence x10
L13:
	ldr x10, [x1, #0x8]
	add x9, x10, x9
	ldaddalb w2, w9, x9
	orr w11, wzr, #0x8
	uxtw x11, w11
	add x12, x1, #0x10
	ldar x12, x12
	add x13, x11, #0x2
	subs xzr, x12, x13
	mov x12, x0
	b.hs #0x34, (L12)
	movz x13, #0x4, lsl 0
	str w13, [x12]
	mov x13, sp
	str x13, [x12, #0x38]
	adr x13, #0x0
	str x13, [x12, #0x30]
	exit_sequence x12
L12:
	add x11, x10, x11
	ands xzr, x11, #0x1
	mov x12, x0
	b.eq #0x34, (L11)
	movz x13, #0x17, lsl 0
	str w13, [x12]
	mov x13, sp
	str x13, [x12, #0x38]
	adr x13, #0x0
	str x13, [x12, #0x30]
	exit_sequence x12
L11:
	ldaddalh w3, w11, x11
	orr w12, wzr, #0x10
	uxtw x12, w12
	add x13, x1, #0x10
	ldar x13, x13
	add x14, x12, #0x4
	subs xzr, x13, x14
	mov x13, x0
	b.hs #0x34, (L10)
	movz x14, #0x4, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L10:
	add x12, x10, x12
	ands xzr, x12, #0x3
	mov x13, x0
	b.eq #0x34, (L9)
	movz x14, #0x17, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L9:
	ldaddal w4, w2, x12
	orr w12, wzr, #0x18
	uxtw x12, w12
	add x13, x1, #0x10
	ldar x13, x13
	add x14, x12, #0x1
	subs xzr, x13, x14
	mov x13, x0
	b.hs #0x34, (L8)
	movz x14, #0x4, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L8:
	add x12, x10, x12
	ldaddalb w5, w3, x12
	orr w12, wzr, #0x20
	uxtw x12, w12
	add x13, x1, #0x10
	ldar x13, x13
	add x14, x12, #0x2
	subs xzr, x13, x14
	mov x13, x0
	b.hs #0x34, (L7)
	movz x14, #0x4, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L7:
	add x12, x10, x12
	ands xzr, x12, #0x1
	mov x13, x0
	b.eq #0x34, (L6)
	movz x14, #0x17, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L6:
	ldaddalh w6, w4, x12
	movz w12, #0x28, lsl 0
	uxtw x12, w12
	add x13, x1, #0x10
	ldar x13, x13
	add x14, x12, #0x4
	subs xzr, x13, x14
	mov x13, x0
	b.hs #0x34, (L5)
	movz x14, #0x4, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L5:
	add x12, x10, x12
	ands xzr, x12, #0x3
	mov x13, x0
	b.eq #0x34, (L4)
	movz x14, #0x17, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L4:
	ldaddal w7, w5, x12
	orr w12, wzr, #0x30
	uxtw x12, w12
	add x13, x1, #0x10
	ldar x13, x13
	add x14, x12, #0x8
	subs xzr, x13, x14
	mov x13, x0
	b.hs #0x34, (L3)
	movz x14, #0x4, lsl 0
	str w14, [x13]
	mov x14, sp
	str x14, [x13, #0x38]
	adr x14, #0x0
	str x14, [x13, #0x30]
	exit_sequence x13
L3:
	add x10, x10, x12
	ands xzr, x10, #0x7
	b.eq #0x34, (L2)
	movz x12, #0x17, lsl 0
	str w12, [x0]
	mov x12, sp
	str x12, [x0, #0x38]
	adr x12, #0x0
	str x12, [x0, #0x30]
	exit_sequence x0
L2:
	ldaddal x8, x6, x10
	mov x1, x11
	mov x0, x9
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	add sp, sp, #0x10
	ret
`,
		},
		{
			name: "icmp_and_zero",
			m:    testcases.IcmpAndZero.Module,
			afterFinalizeAMD64: `
L0 (SSA Block: blk0):
	pushq %rbp
	movq %rsp, %rbp
	testl %edi, %ecx
	jnz L2
L1 (SSA Block: blk1):
	movl $1, %eax
	movq %rbp, %rsp
	popq %rbp
	ret
L2 (SSA Block: blk2):
	xor %rax, %rax
	movq %rbp, %rsp
	popq %rbp
	ret
`,
			afterFinalizeARM64: `
L0 (SSA Block: blk0):
	stp x30, xzr, [sp, #-0x10]!
	str xzr, [sp, #-0x10]!
	ands wzr, w2, w3
	b.ne #0x14, (L2)
L1 (SSA Block: blk1):
	orr w0, wzr, #0x1
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
L2 (SSA Block: blk2):
	mov x0, xzr
	add sp, sp, #0x10
	ldr x30, [sp], #0x10
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var exp string
			switch runtime.GOARCH {
			case "arm64":
				exp = tc.afterFinalizeARM64
			case "amd64":
				if tc.afterFinalizeAMD64 != "" {
					exp = tc.afterFinalizeAMD64
				} else {
					t.Skip()
				}
			default:
				t.Fail()
			}

			err := tc.m.Validate(api.CoreFeaturesV2 | experimental.CoreFeaturesThreads)
			require.NoError(t, err)

			ssab := ssa.NewBuilder()
			offset := wazevoapi.NewModuleContextOffsetData(tc.m, false)
			fc := frontend.NewFrontendCompiler(tc.m, ssab, &offset, false, false, false)
			machine := newMachine()
			machine.DisableStackCheck()
			be := backend.NewCompiler(context.Background(), machine, ssab)

			// Lowers the Wasm to SSA.
			typeIndex := tc.m.FunctionSection[tc.targetIndex]
			code := &tc.m.CodeSection[tc.targetIndex]
			fc.Init(tc.targetIndex, typeIndex, &tc.m.TypeSection[typeIndex], code.LocalTypes, code.Body, false, 0)
			fc.LowerToSSA()
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
			case "amd64":
				if tc.afterLoweringAMD64 != "" {
					require.Equal(t, tc.afterLoweringAMD64, be.Format())
				}
			default:
				t.Fail()
			}

			be.RegAlloc()
			if verbose {
				fmt.Println("============ regalloc result ============")
				fmt.Println(be.Format())
			}

			err = be.Finalize(context.Background())
			require.NoError(t, err)
			if verbose {
				fmt.Println("============ finalization result ============")
				fmt.Println(be.Format())
			}

			require.Equal(t, exp, be.Format())
		})
	}
}
