package jit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"
)

func Test_simdRegisterForScalarFloatRegister(t *testing.T) {
	require.Equal(t, int16(arm64.REG_V0), simdRegisterForScalarFloatRegister(arm64.REG_F0))
	require.Equal(t, int16(arm64.REG_V30), simdRegisterForScalarFloatRegister(arm64.REG_F30))
}
