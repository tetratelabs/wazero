package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestArchContextOffsetInArm64Engine(t *testing.T) {
	var ctx callEngine
	require.Equal(t, int(unsafe.Offsetof(ctx.compilerCallReturnAddress)), arm64CallEngineArchContextCompilerCallReturnAddressOffset, "fix consts in compiler_arm64.s")
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum32BitSignedInt)), arm64CallEngineArchContextMinimum32BitSignedIntOffset)
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum64BitSignedInt)), arm64CallEngineArchContextMinimum64BitSignedIntOffset)
}
