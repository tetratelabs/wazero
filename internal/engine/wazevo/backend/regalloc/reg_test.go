package regalloc

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestRegTypeOf(t *testing.T) {
	require.Equal(t, RegTypeInt, RegTypeOf(ssa.TypeI32))
	require.Equal(t, RegTypeInt, RegTypeOf(ssa.TypeI64))
	require.Equal(t, RegTypeFloat, RegTypeOf(ssa.TypeF32))
	require.Equal(t, RegTypeFloat, RegTypeOf(ssa.TypeF64))
}

func TestVReg_String(t *testing.T) {
	require.Equal(t, "v0?", VReg(0).String())
	require.Equal(t, "v100?", VReg(100).String())
	require.Equal(t, "r5", FromRealReg(5, RegTypeInt).String())
}

func Test_FromRealReg(t *testing.T) {
	r := FromRealReg(5, RegTypeInt)
	require.Equal(t, RealReg(5), r.RealReg())
	require.Equal(t, VRegID(5), r.ID())
}
