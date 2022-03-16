package jit

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestArchContextOffsetInEngine(t *testing.T) {
	var ctx callEngine
	require.Equal(t, int(unsafe.Offsetof(ctx.jitCallReturnAddress)), callEngineArchContextJITCallReturnAddressOffset, "fix consts in jit_arm64.s")
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum32BitSignedInt)), callEngineArchContextMinimum32BitSignedIntOffset)
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum64BitSignedInt)), callEngineArchContextMinimum64BitSignedIntOffset)
}
