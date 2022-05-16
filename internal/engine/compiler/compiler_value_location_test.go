package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_isIntRegister(t *testing.T) {
	for _, r := range unreservedGeneralPurposeIntRegisters {
		require.True(t, isIntRegister(r))
	}
}

func Test_isFloatRegister(t *testing.T) {
	for _, r := range unreservedGeneralPurposeFloatRegisters {
		require.True(t, isFloatRegister(r))
	}
}

func TestValueLocationStack_basic(t *testing.T) {
	s := newValueLocationStack()
	// Push stack value.
	loc := s.pushValueLocationOnStack()
	require.Equal(t, uint64(1), s.sp)
	require.Equal(t, uint64(0), loc.stackPointer)
	// Push the register value.
	tmpReg := unreservedGeneralPurposeIntRegisters[0]
	loc = s.pushValueLocationOnRegister(tmpReg)
	require.Equal(t, uint64(2), s.sp)
	require.Equal(t, uint64(1), loc.stackPointer)
	require.Equal(t, tmpReg, loc.register)
	// markRegisterUsed.
	tmpReg2 := unreservedGeneralPurposeIntRegisters[1]
	s.markRegisterUsed(tmpReg2)
	require.NotNil(t, s.usedRegisters[tmpReg2], tmpReg2)
	// releaseRegister.
	s.releaseRegister(loc)
	require.Equal(t, s.usedRegisters[loc.register], struct{}{}, "expected %v to not contain %v", s.usedRegisters, loc.register)
	require.Equal(t, asm.NilRegister, loc.register)
	// Clone.
	cloned := s.clone()
	require.Equal(t, s.usedRegisters, cloned.usedRegisters)
	require.Equal(t, len(s.stack), len(cloned.stack))
	require.Equal(t, s.sp, cloned.sp)
	for i := 0; i < int(s.sp); i++ {
		actual, exp := s.stack[i], cloned.stack[i]
		require.NotEqual(t, uintptr(unsafe.Pointer(exp)), uintptr(unsafe.Pointer(actual)))
	}
	// Check the max stack pointer.
	for i := 0; i < 1000; i++ {
		s.pushValueLocationOnStack()
	}
	for i := 0; i < 1000; i++ {
		s.pop()
	}
	require.Equal(t, uint64(1001), s.stackPointerCeil)
}

func TestValueLocationStack_takeFreeRegister(t *testing.T) {
	s := newValueLocationStack()
	// For int registers.
	r, ok := s.takeFreeRegister(generalPurposeRegisterTypeInt)
	require.True(t, ok)
	require.True(t, isIntRegister(r))
	// Mark all the int registers used.
	for _, r := range unreservedGeneralPurposeIntRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for int.
	_, ok = s.takeFreeRegister(generalPurposeRegisterTypeInt)
	require.False(t, ok)
	// But we still should be able to take float regs.
	r, ok = s.takeFreeRegister(generalPurposeRegisterTypeFloat)
	require.True(t, ok)
	require.True(t, isFloatRegister(r))
	// Mark all the float registers used.
	for _, r := range unreservedGeneralPurposeFloatRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for floats.
	_, ok = s.takeFreeRegister(generalPurposeRegisterTypeFloat)
	require.False(t, ok)
}

func TestValueLocationStack_takeStealTargetFromUsedRegister(t *testing.T) {
	s := newValueLocationStack()
	intReg := unreservedGeneralPurposeIntRegisters[0]
	intLocation := &valueLocation{register: intReg}
	floatReg := unreservedGeneralPurposeFloatRegisters[0]
	floatLocation := &valueLocation{register: floatReg}
	s.push(intLocation)
	s.push(floatLocation)
	// Take for float.
	target, ok := s.takeStealTargetFromUsedRegister(generalPurposeRegisterTypeFloat)
	require.True(t, ok)
	require.Equal(t, floatLocation, target)
	// Take for ints.
	target, ok = s.takeStealTargetFromUsedRegister(generalPurposeRegisterTypeInt)
	require.True(t, ok)
	require.Equal(t, intLocation, target)
	// Pop float value.
	popped := s.pop()
	require.Equal(t, floatLocation, popped)
	// Now we cannot find the steal target.
	target, ok = s.takeStealTargetFromUsedRegister(generalPurposeRegisterTypeFloat)
	require.False(t, ok)
	require.Nil(t, target)
	// Pop int value.
	popped = s.pop()
	require.Equal(t, intLocation, popped)
	// Now we cannot find the steal target.
	target, ok = s.takeStealTargetFromUsedRegister(generalPurposeRegisterTypeInt)
	require.False(t, ok)
	require.Nil(t, target)
}
