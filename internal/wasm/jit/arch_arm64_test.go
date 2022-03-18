package jit

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestArchContextOffsetInArm64Engine(t *testing.T) {
	var ctx callEngine
	require.Equal(t, int(unsafe.Offsetof(ctx.jitCallReturnAddress)), arm64CallEngineArchContextJITCallReturnAddressOffset, "fix consts in jit_arm64.s")
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum32BitSignedInt)), arm64CallEngineArchContextMinimum32BitSignedIntOffset)
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum64BitSignedInt)), arm64CallEngineArchContextMinimum64BitSignedIntOffset)
}
