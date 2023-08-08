package frontend

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/testcases"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestCompiler_LowerToSSA(t *testing.T) {
	// Most of the logic should look similar to Cranelift's Wasm frontend, so when you want to see
	// what output should look like, you can run:
	// `~/wasmtime/target/debug/clif-util wasm --target aarch64-apple-darwin testcase.wat -p -t`
	for _, tc := range []struct {
		name string
		// m is the *wasm.Module to be compiled in this test.
		m *wasm.Module
		// targetIndex is the index of a local function to be compiled in this test.
		targetIndex wasm.Index
		// exp is the *unoptimized* expected SSA IR for the function m.FunctionSection[targetIndex].
		exp string
		// expAfterOpt is not empty when we want to check the result after optimization passes.
		expAfterOpt string
	}{
		{
			name: "empty", m: testcases.Empty.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk_ret
`,
		},
		{
			name: "unreachable", m: testcases.Unreachable.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0)
	v2:i32 = Iconst_32 0x2
	Store v2, exec_ctx, 0x0
	Trap exec_ctx
`,
		},
		{
			name: testcases.OnlyReturn.Name, m: testcases.OnlyReturn.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Return
`,
		},
		{
			name: "params", m: testcases.Params.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:f32, v4:f64)
	Return
`,
		},
		{
			name: "add/sub params return", m: testcases.AddSubParamsReturn.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	v4:i32 = Iadd v2, v3
	v5:i32 = Isub v4, v2
	Jump blk_ret, v5
`,
		},
		{
			name: "locals", m: testcases.Locals.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	v3:i64 = Iconst_64 0x0
	v4:f32 = F32const 0.000000
	v5:f64 = F64const 0.000000
	Jump blk_ret
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk_ret
`,
		},
		{
			name: "locals + params", m: testcases.LocalsParams.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i64, v3:f32, v4:f64)
	v5:i32 = Iconst_32 0x0
	v6:i64 = Iconst_64 0x0
	v7:f32 = F32const 0.000000
	v8:f64 = F64const 0.000000
	v9:i64 = Iadd v2, v2
	v10:i64 = Isub v9, v2
	v11:f32 = Fadd v3, v3
	v12:f32 = Fsub v11, v3
	v13:f32 = Fmul v12, v3
	v14:f32 = Fdiv v13, v3
	v15:f32 = Fmax v14, v3
	v16:f32 = Fmin v15, v3
	v17:f64 = Fadd v4, v4
	v18:f64 = Fsub v17, v4
	v19:f64 = Fmul v18, v4
	v20:f64 = Fdiv v19, v4
	v21:f64 = Fmax v20, v4
	v22:f64 = Fmin v21, v4
	Jump blk_ret, v10, v16, v22
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i64, v3:f32, v4:f64)
	v9:i64 = Iadd v2, v2
	v10:i64 = Isub v9, v2
	v11:f32 = Fadd v3, v3
	v12:f32 = Fsub v11, v3
	v13:f32 = Fmul v12, v3
	v14:f32 = Fdiv v13, v3
	v15:f32 = Fmax v14, v3
	v16:f32 = Fmin v15, v3
	v17:f64 = Fadd v4, v4
	v18:f64 = Fsub v17, v4
	v19:f64 = Fmul v18, v4
	v20:f64 = Fdiv v19, v4
	v21:f64 = Fmax v20, v4
	v22:f64 = Fmin v21, v4
	Jump blk_ret, v10, v16, v22
`,
		},
		{
			name: "locals + params + return", m: testcases.LocalParamReturn.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	v3:i32 = Iconst_32 0x0
	Jump blk_ret, v2, v3
`,
		},
		{
			name: "swap param and return", m: testcases.SwapParamAndReturn.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	Jump blk_ret, v3, v2
`,
		},
		{
			name: "swap params and return", m: testcases.SwapParamsAndReturn.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	Jump blk1

blk1: () <-- (blk0)
	Jump blk_ret, v3, v2
`,
		},
		{
			name: "block - br", m: testcases.BlockBr.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	v3:i64 = Iconst_64 0x0
	v4:f32 = F32const 0.000000
	v5:f64 = F64const 0.000000
	Jump blk1

blk1: () <-- (blk0)
	Jump blk_ret
`,
		},
		{
			name: "block - br_if", m: testcases.BlockBrIf.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	Brnz v2, blk1
	Jump blk2

blk1: () <-- (blk0)
	Jump blk_ret

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk2)
	v3:i32 = Iconst_32 0x2
	Store v3, exec_ctx, 0x0
	Trap exec_ctx
`,
		},
		{
			name: "loop - br", m: testcases.LoopBr.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0,blk1)
	Jump blk1

blk2: ()
	Jump blk_ret
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0,blk1)
	Jump blk1
`,
		},
		{
			name: "loop - br_if", m: testcases.LoopBrIf.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0,blk1)
	v2:i32 = Iconst_32 0x1
	Brnz v2, blk1
	Jump blk3

blk2: ()
	Jump blk_ret

blk3: () <-- (blk1)
	Return
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0,blk1)
	v2:i32 = Iconst_32 0x1
	Brnz v2, blk1
	Jump blk3

blk3: () <-- (blk1)
	Return
`,
		},
		{
			name: "block - block - br", m: testcases.BlockBlockBr.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	v3:i64 = Iconst_64 0x0
	v4:f32 = F32const 0.000000
	v5:f64 = F64const 0.000000
	Jump blk1

blk1: () <-- (blk0,blk2)
	Jump blk_ret

blk2: ()
	Jump blk1
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64)
	Jump blk1

blk1: () <-- (blk0)
	Jump blk_ret
`,
		},
		{
			name: "if without else", m: testcases.IfWithoutElse.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Jump blk3

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk1,blk2)
	Jump blk_ret
`,
		},
		{
			name: "if-else", m: testcases.IfElse.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Jump blk3

blk2: () <-- (blk0)
	Jump blk_ret

blk3: () <-- (blk1)
	Jump blk_ret
`,
		},
		{
			name: "single predecessor local refs", m: testcases.SinglePredecessorLocalRefs.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	v3:i32 = Iconst_32 0x0
	v4:i32 = Iconst_32 0x0
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Return v4

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk2)
	Jump blk_ret, v2
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64)
	v2:i32 = Iconst_32 0x0
	v4:i32 = Iconst_32 0x0
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Return v4

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk2)
	Jump blk_ret, v2
`,
		},
		{
			name: "multi predecessors local ref",
			m:    testcases.MultiPredecessorLocalRef.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	v4:i32 = Iconst_32 0x0
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Jump blk3, v2

blk2: () <-- (blk0)
	Jump blk3, v3

blk3: (v5:i32) <-- (blk1,blk2)
	Jump blk_ret, v5
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	Brz v2, blk2
	Jump blk1

blk1: () <-- (blk0)
	Jump blk3, v2

blk2: () <-- (blk0)
	Jump blk3, v3

blk3: (v5:i32) <-- (blk1,blk2)
	Jump blk_ret, v5
`,
		},
		{
			name: "reference value from unsealed block",
			m:    testcases.ReferenceValueFromUnsealedBlock.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	v3:i32 = Iconst_32 0x0
	Jump blk1, v2

blk1: (v4:i32) <-- (blk0)
	Return v4

blk2: (v5:i32)
	Jump blk_ret, v5
`,
		},
		{
			name: "reference value from unsealed block - #2",
			m:    testcases.ReferenceValueFromUnsealedBlock2.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	Jump blk1, v2

blk1: (v3:i32) <-- (blk0,blk1)
	Brnz v3, blk1, v3
	Jump blk4

blk2: () <-- (blk3)
	v4:i32 = Iconst_32 0x0
	Jump blk_ret, v4

blk3: () <-- (blk4)
	Jump blk2

blk4: () <-- (blk1)
	Jump blk3
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	Jump blk1

blk1: () <-- (blk0,blk1)
	Brnz v2, blk1
	Jump blk4

blk2: () <-- (blk3)
	v4:i32 = Iconst_32 0x0
	Jump blk_ret, v4

blk3: () <-- (blk4)
	Jump blk2

blk4: () <-- (blk1)
	Jump blk3
`,
		},
		{
			name: "reference value from unsealed block - #3",
			m:    testcases.ReferenceValueFromUnsealedBlock3.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	Jump blk1, v2

blk1: (v3:i32) <-- (blk0,blk3)
	Brnz v3, blk_ret
	Jump blk4

blk2: ()
	Jump blk_ret

blk3: () <-- (blk4)
	v4:i32 = Iconst_32 0x1
	Jump blk1, v4

blk4: () <-- (blk1)
	Jump blk3
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	Jump blk1, v2

blk1: (v3:i32) <-- (blk0,blk3)
	Brnz v3, blk_ret
	Jump blk4

blk3: () <-- (blk4)
	v4:i32 = Iconst_32 0x1
	Jump blk1, v4

blk4: () <-- (blk1)
	Jump blk3
`,
		},
		{
			name: "call",
			m:    testcases.Call.Module,
			exp: `
signatures:
	sig1: i64i64_i32
	sig2: i64i64i32i32_i32
	sig3: i64i64i32_i32i32

blk0: (exec_ctx:i64, module_ctx:i64)
	Store module_ctx, exec_ctx, 0x8
	v2:i32 = Call f1:sig1, exec_ctx, module_ctx
	v3:i32 = Iconst_32 0x5
	Store module_ctx, exec_ctx, 0x8
	v4:i32 = Call f2:sig2, exec_ctx, module_ctx, v2, v3
	Store module_ctx, exec_ctx, 0x8
	v5:i32, v6:i32 = Call f3:sig3, exec_ctx, module_ctx, v4
	Jump blk_ret, v5, v6
`,
		},
		{
			name: "call_many_params",
			m:    testcases.CallManyParams.Module,
			exp: `
signatures:
	sig1: i64i64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64_v

blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64)
	Store module_ctx, exec_ctx, 0x8
	Call f1:sig1, exec_ctx, module_ctx, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5
	Jump blk_ret
`,
		},
		{
			name: "call_many_returns",
			m:    testcases.CallManyReturns.Module,
			exp: `
signatures:
	sig0: i64i64i32i64f32f64_i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64i32i64f32f64

blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64)
	Store module_ctx, exec_ctx, 0x8
	v6:i32, v7:i64, v8:f32, v9:f64, v10:i32, v11:i64, v12:f32, v13:f64, v14:i32, v15:i64, v16:f32, v17:f64, v18:i32, v19:i64, v20:f32, v21:f64, v22:i32, v23:i64, v24:f32, v25:f64, v26:i32, v27:i64, v28:f32, v29:f64, v30:i32, v31:i64, v32:f32, v33:f64, v34:i32, v35:i64, v36:f32, v37:f64, v38:i32, v39:i64, v40:f32, v41:f64, v42:i32, v43:i64, v44:f32, v45:f64 = Call f1:sig0, exec_ctx, module_ctx, v2, v3, v4, v5
	Jump blk_ret, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15, v16, v17, v18, v19, v20, v21, v22, v23, v24, v25, v26, v27, v28, v29, v30, v31, v32, v33, v34, v35, v36, v37, v38, v39, v40, v41, v42, v43, v44, v45
`,
		},
		{
			name: "integer comparisons", m: testcases.IntegerComparisons.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32, v4:i64, v5:i64)
	v6:i32 = Icmp eq, v2, v3
	v7:i32 = Icmp eq, v4, v5
	v8:i32 = Icmp neq, v2, v3
	v9:i32 = Icmp neq, v4, v5
	v10:i32 = Icmp lt_s, v2, v3
	v11:i32 = Icmp lt_s, v4, v5
	v12:i32 = Icmp lt_u, v2, v3
	v13:i32 = Icmp lt_u, v4, v5
	v14:i32 = Icmp gt_s, v2, v3
	v15:i32 = Icmp gt_s, v4, v5
	v16:i32 = Icmp gt_u, v2, v3
	v17:i32 = Icmp gt_u, v4, v5
	v18:i32 = Icmp le_s, v2, v3
	v19:i32 = Icmp le_s, v4, v5
	v20:i32 = Icmp le_u, v2, v3
	v21:i32 = Icmp le_u, v4, v5
	v22:i32 = Icmp ge_s, v2, v3
	v23:i32 = Icmp ge_s, v4, v5
	v24:i32 = Icmp ge_u, v2, v3
	v25:i32 = Icmp ge_u, v4, v5
	Jump blk_ret, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15, v16, v17, v18, v19, v20, v21, v22, v23, v24, v25
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32, v4:i64, v5:i64)
	v6:i32 = Icmp eq, v2, v3
	v7:i32 = Icmp eq, v4, v5
	v8:i32 = Icmp neq, v2, v3
	v9:i32 = Icmp neq, v4, v5
	v10:i32 = Icmp lt_s, v2, v3
	v11:i32 = Icmp lt_s, v4, v5
	v12:i32 = Icmp lt_u, v2, v3
	v13:i32 = Icmp lt_u, v4, v5
	v14:i32 = Icmp gt_s, v2, v3
	v15:i32 = Icmp gt_s, v4, v5
	v16:i32 = Icmp gt_u, v2, v3
	v17:i32 = Icmp gt_u, v4, v5
	v18:i32 = Icmp le_s, v2, v3
	v19:i32 = Icmp le_s, v4, v5
	v20:i32 = Icmp le_u, v2, v3
	v21:i32 = Icmp le_u, v4, v5
	v22:i32 = Icmp ge_s, v2, v3
	v23:i32 = Icmp ge_s, v4, v5
	v24:i32 = Icmp ge_u, v2, v3
	v25:i32 = Icmp ge_u, v4, v5
	Jump blk_ret, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15, v16, v17, v18, v19, v20, v21, v22, v23, v24, v25
`,
		},
		{
			name: "integer shift", m: testcases.IntegerShift.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32, v4:i64, v5:i64)
	v6:i32 = Ishl v2, v3
	v7:i32 = Iconst_32 0x1f
	v8:i32 = Ishl v2, v7
	v9:i64 = Ishl v4, v5
	v10:i64 = Iconst_64 0x20
	v11:i64 = Ishl v4, v10
	v12:i32 = Ushr v2, v3
	v13:i32 = Iconst_32 0x1f
	v14:i32 = Ushr v2, v13
	v15:i64 = Ushr v4, v5
	v16:i64 = Iconst_64 0x20
	v17:i64 = Ushr v4, v16
	v18:i32 = Sshr v2, v3
	v19:i32 = Iconst_32 0x1f
	v20:i32 = Sshr v2, v19
	v21:i64 = Sshr v4, v5
	v22:i64 = Iconst_64 0x20
	v23:i64 = Sshr v4, v22
	Jump blk_ret, v6, v8, v9, v11, v12, v14, v15, v17, v18, v20, v21, v23
`,
		},
		{
			name: "integer extension", m: testcases.IntegerExtensions.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64)
	v4:i64 = SExtend v2, 32->64
	v5:i64 = UExtend v2, 32->64
	v6:i64 = SExtend v3, 8->64
	v7:i64 = SExtend v3, 16->64
	v8:i64 = SExtend v3, 32->64
	v9:i32 = SExtend v2, 8->32
	v10:i32 = SExtend v2, 16->32
	Jump blk_ret, v4, v5, v6, v7, v8, v9, v10
`,
		},
		{
			name: "float comparisons", m: testcases.FloatComparisons.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:f32, v3:f32, v4:f64, v5:f64)
	v6:i32 = Fcmp eq, v2, v3
	v7:i32 = Fcmp neq, v2, v3
	v8:i32 = Fcmp lt, v2, v3
	v9:i32 = Fcmp gt, v2, v3
	v10:i32 = Fcmp le, v2, v3
	v11:i32 = Fcmp ge, v2, v3
	v12:i32 = Fcmp eq, v4, v5
	v13:i32 = Fcmp neq, v4, v5
	v14:i32 = Fcmp lt, v4, v5
	v15:i32 = Fcmp gt, v4, v5
	v16:i32 = Fcmp le, v4, v5
	v17:i32 = Fcmp ge, v4, v5
	Jump blk_ret, v6, v7, v8, v9, v10, v11, v12, v13, v14, v15, v16, v17
`,
		},
		{
			name: "loop with param and results", m: testcases.LoopBrWithParamResults.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	Jump blk1, v2, v3

blk1: (v4:i32,v5:i32) <-- (blk0,blk1)
	v7:i32 = Iconst_32 0x1
	Brnz v7, blk1, v4, v5
	Jump blk3

blk2: (v6:i32) <-- (blk3)
	Jump blk_ret, v6

blk3: () <-- (blk1)
	Jump blk2, v4
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i32)
	Jump blk1

blk1: () <-- (blk0,blk1)
	v7:i32 = Iconst_32 0x1
	Brnz v7, blk1
	Jump blk3

blk2: () <-- (blk3)
	Jump blk_ret, v2

blk3: () <-- (blk1)
	Jump blk2
`,
		},
		{
			name:        "many_params_small_results",
			m:           testcases.ManyParamsSmallResults.Module,
			targetIndex: 0,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64, v6:i32, v7:i64, v8:f32, v9:f64, v10:i32, v11:i64, v12:f32, v13:f64, v14:i32, v15:i64, v16:f32, v17:f64, v18:i32, v19:i64, v20:f32, v21:f64, v22:i32, v23:i64, v24:f32, v25:f64, v26:i32, v27:i64, v28:f32, v29:f64, v30:i32, v31:i64, v32:f32, v33:f64, v34:i32, v35:i64, v36:f32, v37:f64, v38:i32, v39:i64, v40:f32, v41:f64)
	Jump blk_ret, v2, v11, v20, v29
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64, v6:i32, v7:i64, v8:f32, v9:f64, v10:i32, v11:i64, v12:f32, v13:f64, v14:i32, v15:i64, v16:f32, v17:f64, v18:i32, v19:i64, v20:f32, v21:f64, v22:i32, v23:i64, v24:f32, v25:f64, v26:i32, v27:i64, v28:f32, v29:f64, v30:i32, v31:i64, v32:f32, v33:f64, v34:i32, v35:i64, v36:f32, v37:f64, v38:i32, v39:i64, v40:f32, v41:f64)
	Jump blk_ret, v2, v11, v20, v29
`,
		},
		{
			name:        "small_params_many_results",
			m:           testcases.SmallParamsManyResults.Module,
			targetIndex: 0,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64)
	Jump blk_ret, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64)
	Jump blk_ret, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5, v2, v3, v4, v5
`,
		},
		{
			name:        "many_params_many_results",
			m:           testcases.ManyParamsManyResults.Module,
			targetIndex: 0,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64, v6:i32, v7:i64, v8:f32, v9:f64, v10:i32, v11:i64, v12:f32, v13:f64, v14:i32, v15:i64, v16:f32, v17:f64, v18:i32, v19:i64, v20:f32, v21:f64, v22:i32, v23:i64, v24:f32, v25:f64, v26:i32, v27:i64, v28:f32, v29:f64, v30:i32, v31:i64, v32:f32, v33:f64, v34:i32, v35:i64, v36:f32, v37:f64, v38:i32, v39:i64, v40:f32, v41:f64)
	Jump blk_ret, v41, v40, v39, v38, v37, v36, v35, v34, v33, v32, v31, v30, v29, v28, v27, v26, v25, v24, v23, v22, v21, v20, v19, v18, v17, v16, v15, v14, v13, v12, v11, v10, v9, v8, v7, v6, v5, v4, v3, v2
`,
			expAfterOpt: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:i64, v4:f32, v5:f64, v6:i32, v7:i64, v8:f32, v9:f64, v10:i32, v11:i64, v12:f32, v13:f64, v14:i32, v15:i64, v16:f32, v17:f64, v18:i32, v19:i64, v20:f32, v21:f64, v22:i32, v23:i64, v24:f32, v25:f64, v26:i32, v27:i64, v28:f32, v29:f64, v30:i32, v31:i64, v32:f32, v33:f64, v34:i32, v35:i64, v36:f32, v37:f64, v38:i32, v39:i64, v40:f32, v41:f64)
	Jump blk_ret, v41, v40, v39, v38, v37, v36, v35, v34, v33, v32, v31, v30, v29, v28, v27, v26, v25, v24, v23, v22, v21, v20, v19, v18, v17, v16, v15, v14, v13, v12, v11, v10, v9, v8, v7, v6, v5, v4, v3, v2
`,
		},
		{
			name: "many_middle_values",
			m:    testcases.ManyMiddleValues.Module,
			exp: `
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32, v3:f32)
	v4:i32 = Iconst_32 0x1
	v5:i32 = Imul v2, v4
	v6:i32 = Iconst_32 0x2
	v7:i32 = Imul v2, v6
	v8:i32 = Iconst_32 0x3
	v9:i32 = Imul v2, v8
	v10:i32 = Iconst_32 0x4
	v11:i32 = Imul v2, v10
	v12:i32 = Iconst_32 0x5
	v13:i32 = Imul v2, v12
	v14:i32 = Iconst_32 0x6
	v15:i32 = Imul v2, v14
	v16:i32 = Iconst_32 0x7
	v17:i32 = Imul v2, v16
	v18:i32 = Iconst_32 0x8
	v19:i32 = Imul v2, v18
	v20:i32 = Iconst_32 0x9
	v21:i32 = Imul v2, v20
	v22:i32 = Iconst_32 0xa
	v23:i32 = Imul v2, v22
	v24:i32 = Iconst_32 0xb
	v25:i32 = Imul v2, v24
	v26:i32 = Iconst_32 0xc
	v27:i32 = Imul v2, v26
	v28:i32 = Iconst_32 0xd
	v29:i32 = Imul v2, v28
	v30:i32 = Iconst_32 0xe
	v31:i32 = Imul v2, v30
	v32:i32 = Iconst_32 0xf
	v33:i32 = Imul v2, v32
	v34:i32 = Iconst_32 0x10
	v35:i32 = Imul v2, v34
	v36:i32 = Iconst_32 0x11
	v37:i32 = Imul v2, v36
	v38:i32 = Iconst_32 0x12
	v39:i32 = Imul v2, v38
	v40:i32 = Iconst_32 0x13
	v41:i32 = Imul v2, v40
	v42:i32 = Iconst_32 0x14
	v43:i32 = Imul v2, v42
	v44:i32 = Iadd v41, v43
	v45:i32 = Iadd v39, v44
	v46:i32 = Iadd v37, v45
	v47:i32 = Iadd v35, v46
	v48:i32 = Iadd v33, v47
	v49:i32 = Iadd v31, v48
	v50:i32 = Iadd v29, v49
	v51:i32 = Iadd v27, v50
	v52:i32 = Iadd v25, v51
	v53:i32 = Iadd v23, v52
	v54:i32 = Iadd v21, v53
	v55:i32 = Iadd v19, v54
	v56:i32 = Iadd v17, v55
	v57:i32 = Iadd v15, v56
	v58:i32 = Iadd v13, v57
	v59:i32 = Iadd v11, v58
	v60:i32 = Iadd v9, v59
	v61:i32 = Iadd v7, v60
	v62:i32 = Iadd v5, v61
	v63:f32 = F32const 1.000000
	v64:f32 = Fmul v3, v63
	v65:f32 = F32const 2.000000
	v66:f32 = Fmul v3, v65
	v67:f32 = F32const 3.000000
	v68:f32 = Fmul v3, v67
	v69:f32 = F32const 4.000000
	v70:f32 = Fmul v3, v69
	v71:f32 = F32const 5.000000
	v72:f32 = Fmul v3, v71
	v73:f32 = F32const 6.000000
	v74:f32 = Fmul v3, v73
	v75:f32 = F32const 7.000000
	v76:f32 = Fmul v3, v75
	v77:f32 = F32const 8.000000
	v78:f32 = Fmul v3, v77
	v79:f32 = F32const 9.000000
	v80:f32 = Fmul v3, v79
	v81:f32 = F32const 10.000000
	v82:f32 = Fmul v3, v81
	v83:f32 = F32const 11.000000
	v84:f32 = Fmul v3, v83
	v85:f32 = F32const 12.000000
	v86:f32 = Fmul v3, v85
	v87:f32 = F32const 13.000000
	v88:f32 = Fmul v3, v87
	v89:f32 = F32const 14.000000
	v90:f32 = Fmul v3, v89
	v91:f32 = F32const 15.000000
	v92:f32 = Fmul v3, v91
	v93:f32 = F32const 16.000000
	v94:f32 = Fmul v3, v93
	v95:f32 = F32const 17.000000
	v96:f32 = Fmul v3, v95
	v97:f32 = F32const 18.000000
	v98:f32 = Fmul v3, v97
	v99:f32 = F32const 19.000000
	v100:f32 = Fmul v3, v99
	v101:f32 = F32const 20.000000
	v102:f32 = Fmul v3, v101
	v103:f32 = Fadd v100, v102
	v104:f32 = Fadd v98, v103
	v105:f32 = Fadd v96, v104
	v106:f32 = Fadd v94, v105
	v107:f32 = Fadd v92, v106
	v108:f32 = Fadd v90, v107
	v109:f32 = Fadd v88, v108
	v110:f32 = Fadd v86, v109
	v111:f32 = Fadd v84, v110
	v112:f32 = Fadd v82, v111
	v113:f32 = Fadd v80, v112
	v114:f32 = Fadd v78, v113
	v115:f32 = Fadd v76, v114
	v116:f32 = Fadd v74, v115
	v117:f32 = Fadd v72, v116
	v118:f32 = Fadd v70, v117
	v119:f32 = Fadd v68, v118
	v120:f32 = Fadd v66, v119
	v121:f32 = Fadd v64, v120
	Jump blk_ret, v62, v121
`,
		},
		{
			name: "recursive_fibonacci", m: testcases.FibonacciRecursive.Module,
			exp: `
signatures:
	sig0: i64i64i32_i32

blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	v3:i32 = Iconst_32 0x2
	v4:i32 = Icmp lt_s, v2, v3
	Brz v4, blk2
	Jump blk1

blk1: () <-- (blk0)
	Return v2

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk2)
	v5:i32 = Iconst_32 0x1
	v6:i32 = Isub v2, v5
	Store module_ctx, exec_ctx, 0x8
	v7:i32 = Call f0:sig0, exec_ctx, module_ctx, v6
	v8:i32 = Iconst_32 0x2
	v9:i32 = Isub v2, v8
	Store module_ctx, exec_ctx, 0x8
	v10:i32 = Call f0:sig0, exec_ctx, module_ctx, v9
	v11:i32 = Iadd v7, v10
	Jump blk_ret, v11
`,
			expAfterOpt: `
signatures:
	sig0: i64i64i32_i32

blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	v3:i32 = Iconst_32 0x2
	v4:i32 = Icmp lt_s, v2, v3
	Brz v4, blk2
	Jump blk1

blk1: () <-- (blk0)
	Return v2

blk2: () <-- (blk0)
	Jump blk3

blk3: () <-- (blk2)
	v5:i32 = Iconst_32 0x1
	v6:i32 = Isub v2, v5
	Store module_ctx, exec_ctx, 0x8
	v7:i32 = Call f0:sig0, exec_ctx, module_ctx, v6
	v8:i32 = Iconst_32 0x2
	v9:i32 = Isub v2, v8
	Store module_ctx, exec_ctx, 0x8
	v10:i32 = Call f0:sig0, exec_ctx, module_ctx, v9
	v11:i32 = Iadd v7, v10
	Jump blk_ret, v11
`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Just in case let's check the test module is valid.
			err := tc.m.Validate(api.CoreFeaturesV2)
			require.NoError(t, err, "invalid test case module!")

			b := ssa.NewBuilder()

			fc := NewFrontendCompiler(tc.m, b)
			typeIndex := tc.m.FunctionSection[tc.targetIndex]
			code := &tc.m.CodeSection[tc.targetIndex]
			fc.Init(tc.targetIndex, &tc.m.TypeSection[typeIndex], code.LocalTypes, code.Body)

			err = fc.LowerToSSA()
			require.NoError(t, err)

			actual := fc.formatBuilder()
			fmt.Println(actual)
			require.Equal(t, tc.exp, actual)

			b.RunPasses()
			if expAfterOpt := tc.expAfterOpt; expAfterOpt != "" {
				actualAfterOpt := fc.formatBuilder()
				fmt.Println(actualAfterOpt)
				require.Equal(t, expAfterOpt, actualAfterOpt)
			}

			// Dry-run without checking the results of LayoutBlocks function.
			b.LayoutBlocks()
		})
	}
}
