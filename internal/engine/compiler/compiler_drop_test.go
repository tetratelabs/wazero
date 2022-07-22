package compiler

import (
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wazeroir"
	"testing"
)

func Test_compileDropRange(t *testing.T) {
	t.Run("nil range", func(t *testing.T) {
		c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
		require.NoError(t, err)

		err = compileDropRange(c, nil)
		require.NoError(t, err)
	})
	t.Run("start at the top", func(t *testing.T) {
		c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
		require.NoError(t, err)

		// Use up all unreserved registers.
		for _, reg := range unreservedGeneralPurposeRegisters {
			c.pushRuntimeValueLocationOnRegister(reg, runtimeValueTypeI32)
		}
		for i, vreg := range unreservedVectorRegisters {
			// Mix and match scalar float and vector values.
			if i%2 == 0 {
				c.pushVectorRuntimeValueLocationOnRegister(vreg)
			} else {
				c.pushRuntimeValueLocationOnRegister(vreg, runtimeValueTypeF32)
			}
		}

		unreservedRegisterTotal := len(unreservedGeneralPurposeRegisters) + len(unreservedVectorRegisters)
		ls := c.runtimeValueLocationStack()
		require.Equal(t, unreservedRegisterTotal, len(ls.usedRegisters))

		// Drop all the values.
		err = compileDropRange(c, &wazeroir.InclusiveRange{Start: 0, End: int(ls.sp - 1)})
		require.NoError(t, err)

		// All the registers must be marked unused.
		require.Equal(t, 0, len(ls.usedRegisters))
		// Also, stack pointer must be zero.
		require.Equal(t, 0, int(ls.sp))
	})
}
