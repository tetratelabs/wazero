//go:build amd64
// +build amd64

package jit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

func Test_isIntRegister(t *testing.T) {
	for _, r := range gpIntRegisters {
		require.True(t, isIntRegister(r))
	}
}

func Test_isFloatRegister(t *testing.T) {
	for _, r := range gpFloatRegisters {
		require.True(t, isFloatRegister(r))
	}
}

func TestValueLocationStack_basic(t *testing.T) {
	s := newValueLocationStack()
	// Push.
	depth := uint64(100)
	s.push(&valueLocation{stackPointer: &depth})
	require.Equal(t, 1, s.sp)
	require.Equal(t, depth, *s.stack[s.sp-1].stackPointer)
	// markRegisterUsed.
	reg := x86.REG_X1
	s.markRegisterUsed(reg)
	require.Contains(t, s.usedRegisters, reg)
	// releaseRegister
	s.releaseRegister(reg)
	require.NotContains(t, s.usedRegisters, reg)
}

func TestValueLocationStack_takeFreeRegister(t *testing.T) {
	s := newValueLocationStack()
	// For int registers.
	r, err := s.takeFreeRegister(gpTypeInt)
	require.NoError(t, err)
	require.True(t, isIntRegister(r))
	// Mark all the int registers used.
	for _, r := range gpIntRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for int.
	_, err = s.takeFreeRegister(gpTypeInt)
	require.Equal(t, errFreeRegisterNotFound, err)
	// But we still should be able to take float regs.
	r, err = s.takeFreeRegister(gpTypeFloat)
	require.NoError(t, err)
	require.True(t, isFloatRegister(r))
	// Mark all the float registers used.
	for _, r := range gpFloatRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for floats.
	_, err = s.takeFreeRegister(gpTypeFloat)
	require.Equal(t, errFreeRegisterNotFound, err)
}

func TestValueLocationStack_takeStealTargetFromUsedRegister(t *testing.T) {
	s := newValueLocationStack()
	intReg := x86.REG_R10
	intLocation := &valueLocation{register: &intReg}
	floatReg := x86.REG_X0
	floatLocation := &valueLocation{register: &floatReg}
	s.push(intLocation)
	s.push(floatLocation)
	// Take for float.
	target := s.takeStealTargetFromUsedRegister(gpTypeFloat)
	require.NotNil(t, target)
	require.Equal(t, floatLocation, target)
	// Take for ints.
	target = s.takeStealTargetFromUsedRegister(gpTypeInt)
	require.NotNil(t, target)
	require.Equal(t, intLocation, target)
	// Pop float value.
	popped := s.pop()
	require.Equal(t, floatLocation, popped)
	// Now we cannot find the steal tareget.
	target = s.takeStealTargetFromUsedRegister(gpTypeFloat)
	require.Nil(t, target)
	// Pop int value.
	popped = s.pop()
	require.Equal(t, intLocation, popped)
	// Now we cannot find the steal tareget.
	target = s.takeStealTargetFromUsedRegister(gpTypeInt)
	require.Nil(t, target)
}
