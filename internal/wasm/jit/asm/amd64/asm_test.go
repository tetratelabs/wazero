package amd64

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

func TestGolangAsmCompatibility(t *testing.T) {
	require.Equal(t, intRegisterIotaBegin, int16(x86.REG_AX))
	require.Equal(t, floatRegisterIotaBegin, int16(x86.REG_X0))
}
