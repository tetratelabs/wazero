package arm64

import (
	"encoding/hex"
	"fmt"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_calleeSavedRegistersPlusLinkRegSorted(t *testing.T) {
	var exp []regalloc.VReg
	for r := range regInfo.CalleeSavedRegisters {
		exp = append(exp, regInfo.RealRegToVReg[r])
	}
	exp = append(exp, regInfo.RealRegToVReg[lr])
	sort.Slice(exp, func(i, j int) bool {
		return exp[i].RealReg() < exp[j].RealReg()
	})

	require.Equal(t, exp, calleeSavedRegistersPlusLinkRegSorted)
}

func TestMachine_CompileGoFunctionTrampoline(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		exitCode             wazevoapi.ExitCode
		sig                  *ssa.Signature
		needModuleContextPtr bool
		exp                  string
	}{
		{
			name:     "go call",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100),
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeF64},
				Results: []ssa.Type{ssa.TypeI32, ssa.TypeI64, ssa.TypeF32, ssa.TypeF64},
			},
			needModuleContextPtr: true,
			exp: `
	str x18, [x0, #0x50]
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
	str x1, [x0, #0x450]
	add x15, x0, #0x458
	str d0, [x15], #0x8
	movz w17, #0x6406, LSL 0
	str w17, [x0]
	mov x27, sp
	str x27, [x0, #0x38]
	adr x27, #0x1c
	str x27, [x0, #0x30]
	exit_sequence w0
	ldr x18, [x0, #0x50]
	ldr x19, [x0, #0x60]
	ldr x20, [x0, #0x70]
	ldr x21, [x0, #0x80]
	ldr x22, [x0, #0x90]
	ldr x23, [x0, #0xa0]
	ldr x24, [x0, #0xb0]
	ldr x25, [x0, #0xc0]
	ldr x26, [x0, #0xd0]
	ldr x28, [x0, #0xe0]
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, x0, #0x458
	ldr w27, [x15], #0x8
	mov w0, w27
	ldr x27, [x15], #0x8
	mov x1, x27
	ldr s17, [x15], #0x8
	mov q0.8b, q17.8b
	ldr d17, [x15], #0x8
	mov q1.8b, q17.8b
	ret
`,
		},
		{
			name:     "go call",
			exitCode: wazevoapi.ExitCodeCallGoFunctionWithIndex(100),
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI64, ssa.TypeI64, ssa.TypeF64, ssa.TypeF64, ssa.TypeI32, ssa.TypeI32},
				Results: []ssa.Type{},
			},
			needModuleContextPtr: true,
			exp: `
	str x18, [x0, #0x50]
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
	str x1, [x0, #0x450]
	add x15, x0, #0x458
	str d0, [x15], #0x8
	str d1, [x15], #0x8
	str x2, [x15], #0x8
	str x3, [x15], #0x8
	movz w17, #0x6406, LSL 0
	str w17, [x0]
	mov x27, sp
	str x27, [x0, #0x38]
	adr x27, #0x1c
	str x27, [x0, #0x30]
	exit_sequence w0
	ldr x18, [x0, #0x50]
	ldr x19, [x0, #0x60]
	ldr x20, [x0, #0x70]
	ldr x21, [x0, #0x80]
	ldr x22, [x0, #0x90]
	ldr x23, [x0, #0xa0]
	ldr x24, [x0, #0xb0]
	ldr x25, [x0, #0xc0]
	ldr x26, [x0, #0xd0]
	ldr x28, [x0, #0xe0]
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, x0, #0x458
	ret
`,
		},
		{
			name:     "grow memory",
			exitCode: wazevoapi.ExitCodeGrowMemory,
			sig: &ssa.Signature{
				Params:  []ssa.Type{ssa.TypeI32, ssa.TypeI32},
				Results: []ssa.Type{ssa.TypeI32},
			},
			exp: `
	str x18, [x0, #0x50]
	str x19, [x0, #0x60]
	str x20, [x0, #0x70]
	str x21, [x0, #0x80]
	str x22, [x0, #0x90]
	str x23, [x0, #0xa0]
	str x24, [x0, #0xb0]
	str x25, [x0, #0xc0]
	str x26, [x0, #0xd0]
	str x28, [x0, #0xe0]
	str x30, [x0, #0xf0]
	str q18, [x0, #0x100]
	str q19, [x0, #0x110]
	str q20, [x0, #0x120]
	str q21, [x0, #0x130]
	str q22, [x0, #0x140]
	str q23, [x0, #0x150]
	str q24, [x0, #0x160]
	str q25, [x0, #0x170]
	str q26, [x0, #0x180]
	str q27, [x0, #0x190]
	str q28, [x0, #0x1a0]
	str q29, [x0, #0x1b0]
	str q30, [x0, #0x1c0]
	str q31, [x0, #0x1d0]
	add x15, x0, #0x458
	str x1, [x15], #0x8
	orr w17, wzr, #0x2
	str w17, [x0]
	mov x27, sp
	str x27, [x0, #0x38]
	adr x27, #0x1c
	str x27, [x0, #0x30]
	exit_sequence w0
	ldr x18, [x0, #0x50]
	ldr x19, [x0, #0x60]
	ldr x20, [x0, #0x70]
	ldr x21, [x0, #0x80]
	ldr x22, [x0, #0x90]
	ldr x23, [x0, #0xa0]
	ldr x24, [x0, #0xb0]
	ldr x25, [x0, #0xc0]
	ldr x26, [x0, #0xd0]
	ldr x28, [x0, #0xe0]
	ldr x30, [x0, #0xf0]
	ldr q18, [x0, #0x100]
	ldr q19, [x0, #0x110]
	ldr q20, [x0, #0x120]
	ldr q21, [x0, #0x130]
	ldr q22, [x0, #0x140]
	ldr q23, [x0, #0x150]
	ldr q24, [x0, #0x160]
	ldr q25, [x0, #0x170]
	ldr q26, [x0, #0x180]
	ldr q27, [x0, #0x190]
	ldr q28, [x0, #0x1a0]
	ldr q29, [x0, #0x1b0]
	ldr q30, [x0, #0x1c0]
	ldr q31, [x0, #0x1d0]
	add x15, x0, #0x458
	ldr w27, [x15], #0x8
	mov w0, w27
	ret
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, m := newSetupWithMockContext()
			m.CompileGoFunctionTrampoline(tc.exitCode, tc.sig, tc.needModuleContextPtr)

			fmt.Println(m.Format())
			require.Equal(t, tc.exp, m.Format())

			m.Encode()
			fmt.Println(hex.EncodeToString(m.compiler.Buf()))
		})
	}
}
