package compiler

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wazeroir"
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

func TestRuntimeValueLocationStack_dropsLivesForInclusiveRange(t *testing.T) {
	tests := []struct {
		v            *runtimeValueLocationStack
		ir           *wazeroir.InclusiveRange
		lives, drops []*runtimeValueLocation
	}{
		{
			v: &runtimeValueLocationStack{
				stack: []*runtimeValueLocation{{register: 0}, {register: 1} /* drop target */, {register: 2}},
				sp:    3,
			},
			ir:    &wazeroir.InclusiveRange{Start: 1, End: 1},
			drops: []*runtimeValueLocation{{register: 1}},
			lives: []*runtimeValueLocation{{register: 2}},
		},
		{
			v: &runtimeValueLocationStack{
				stack: []*runtimeValueLocation{
					{register: 0},
					{register: 1},
					{register: 2}, // drop target
					{register: 3}, // drop target
					{register: 4}, // drop target
					{register: 5},
					{register: 6},
				},
				sp: 7,
			},
			ir:    &wazeroir.InclusiveRange{Start: 2, End: 4},
			drops: []*runtimeValueLocation{{register: 2}, {register: 3}, {register: 4}},
			lives: []*runtimeValueLocation{{register: 5}, {register: 6}},
		},
	}

	for _, tc := range tests {
		actualDrops, actualLives := tc.v.dropsLivesForInclusiveRange(tc.ir)
		require.Equal(t, tc.drops, actualDrops)
		require.Equal(t, tc.lives, actualLives)
	}
}

func Test_getTemporariesForStackedLiveValues(t *testing.T) {
	t.Run("no stacked values", func(t *testing.T) {
		liveValues := []*runtimeValueLocation{{register: 1}, {register: 2}}
		c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
		require.NoError(t, err)

		gpTmp, vecTmp, err := getTemporariesForStackedLiveValues(c, liveValues)
		require.NoError(t, err)

		require.Equal(t, asm.NilRegister, gpTmp)
		require.Equal(t, asm.NilRegister, vecTmp)
	})
	t.Run("general purpose needed", func(t *testing.T) {
		for _, freeRegisterExists := range []bool{false, true} {
			freeRegisterExists := freeRegisterExists
			t.Run(fmt.Sprintf("free register exists=%v", freeRegisterExists), func(t *testing.T) {

				liveValues := []*runtimeValueLocation{
					// Even multiple integer values are alive and on stack,
					// only one general purpose register should be chosen.
					{valueType: runtimeValueTypeI32},
					{valueType: runtimeValueTypeI64},
				}
				c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
				require.NoError(t, err)

				if !freeRegisterExists {
					// Use up all the unreserved gp registers.
					for _, reg := range unreservedGeneralPurposeRegisters {
						c.pushRuntimeValueLocationOnRegister(reg, runtimeValueTypeI32)
					}
					// Ensures actually we used them up all.
					require.Equal(t, len(c.runtimeValueLocationStack().usedRegisters),
						len(unreservedGeneralPurposeRegisters))
				}

				gpTmp, vecTmp, err := getTemporariesForStackedLiveValues(c, liveValues)
				require.NoError(t, err)

				if !freeRegisterExists {
					// At this point, one register should be marked as unused.
					require.Equal(t, len(c.runtimeValueLocationStack().usedRegisters),
						len(unreservedGeneralPurposeRegisters)-1)
				}

				require.NotEqual(t, asm.NilRegister, gpTmp)
				require.Equal(t, asm.NilRegister, vecTmp)
			})
		}
	})

	t.Run("vector needed", func(t *testing.T) {
		for _, freeRegisterExists := range []bool{false, true} {
			freeRegisterExists := freeRegisterExists
			t.Run(fmt.Sprintf("free register exists=%v", freeRegisterExists), func(t *testing.T) {

				liveValues := []*runtimeValueLocation{
					// Even multiple vectors are alive and on stack,
					// only one vector register should be chosen.
					{valueType: runtimeValueTypeF32},
					{valueType: runtimeValueTypeV128Lo},
					{valueType: runtimeValueTypeV128Hi},
					{valueType: runtimeValueTypeV128Lo},
					{valueType: runtimeValueTypeV128Hi},
				}
				c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
				require.NoError(t, err)

				if !freeRegisterExists {
					// Use up all the unreserved gp registers.
					for _, reg := range unreservedVectorRegisters {
						c.pushVectorRuntimeValueLocationOnRegister(reg)
					}
					// Ensures actually we used them up all.
					require.Equal(t, len(c.runtimeValueLocationStack().usedRegisters),
						len(unreservedVectorRegisters))
				}

				gpTmp, vecTmp, err := getTemporariesForStackedLiveValues(c, liveValues)
				require.NoError(t, err)

				if !freeRegisterExists {
					// At this point, one register should be marked as unused.
					require.Equal(t, len(c.runtimeValueLocationStack().usedRegisters),
						len(unreservedVectorRegisters)-1)
				}

				require.Equal(t, asm.NilRegister, gpTmp)
				require.NotEqual(t, asm.NilRegister, vecTmp)
			})
		}
	})
}

func Test_migrateLiveValue(t *testing.T) {
	t.Run("v128.hi", func(t *testing.T) {
		migrateLiveValue(nil, &runtimeValueLocation{valueType: runtimeValueTypeV128Hi}, asm.NilRegister, asm.NilRegister)
	})
	t.Run("already on register", func(t *testing.T) {
		// This case, we don't use tmp registers.
		c, err := newCompiler(nil) // we don't use ir in compileDropRange, so passing nil is fine.
		require.NoError(t, err)

		// Push the dummy values.
		for i := 0; i < 10; i++ {
			_ = c.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
		}

		gpReg := unreservedGeneralPurposeRegisters[0]
		vReg := unreservedVectorRegisters[0]
		c.pushRuntimeValueLocationOnRegister(gpReg, runtimeValueTypeI64)
		c.pushVectorRuntimeValueLocationOnRegister(vReg)

		// Emulate the compileDrop
		ls := c.runtimeValueLocationStack()
		vLive, gpLive := ls.popV128(), ls.pop()
		const dropNum = 5
		ls.sp -= dropNum

		// Migrate these two values.
		migrateLiveValue(c, gpLive, asm.NilRegister, asm.NilRegister)
		migrateLiveValue(c, vLive, asm.NilRegister, asm.NilRegister)

		// Check the new stack location.
		vectorMigrated, gpMigrated := ls.popV128(), ls.pop()
		require.Equal(t, uint64(5), gpMigrated.stackPointer)
		require.Equal(t, uint64(6), vectorMigrated.stackPointer)

		require.Equal(t, gpLive.register, gpMigrated.register)
		require.Equal(t, vLive.register, vectorMigrated.register)
	})
}
