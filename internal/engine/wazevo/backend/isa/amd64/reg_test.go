package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_formatVRegSize(t *testing.T) {
	tests := []struct {
		r   regalloc.VReg
		_64 bool
		exp string
	}{
		// Real registers.
		{r: raxVReg, _64: true, exp: "%rax"},
		{r: raxVReg, _64: false, exp: "%eax"},
		{r: r15VReg, _64: true, exp: "%r15"},
		{r: r15VReg, _64: false, exp: "%r15d"},
		{r: xmm0VReg, _64: true, exp: "%xmm0"},
		{r: xmm0VReg, _64: false, exp: "%xmm0"},
		{r: xmm15VReg, _64: true, exp: "%xmm15"},
		{r: xmm15VReg, _64: false, exp: "%xmm15"},
		// Non-real registers.
		{r: regalloc.VReg(5555).SetRegType(regalloc.RegTypeInt), _64: true, exp: "%r5555?"},
		{r: regalloc.VReg(5555).SetRegType(regalloc.RegTypeInt), _64: false, exp: "%r5555d?"},
		{r: regalloc.VReg(123).SetRegType(regalloc.RegTypeFloat), _64: true, exp: "%xmm123?"},
		{r: regalloc.VReg(432).SetRegType(regalloc.RegTypeFloat), _64: false, exp: "%xmm432?"},
	}
	for _, tt := range tests {
		t.Run(tt.exp, func(t *testing.T) {
			require.Equal(t, tt.exp, formatVRegSized(tt.r, tt._64))
		})
	}
}
