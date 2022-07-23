package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_isIntRegister(t *testing.T) {
	for _, r := range unreservedGeneralPurposeRegisters {
		require.True(t, isGeneralPurposeRegister(r))
	}
}

func Test_isVectorRegister(t *testing.T) {
	for _, r := range unreservedVectorRegisters {
		require.True(t, isVectorRegister(r))
	}
}

func TestRuntimeValueLocationStack_basic(t *testing.T) {
	s := newRuntimeValueLocationStack()
	// Push stack value.
	loc := s.pushRuntimeValueLocationOnStack()
	require.Equal(t, uint64(1), s.sp)
	require.Equal(t, uint64(0), loc.stackPointer)
	// Push the register value.
	tmpReg := unreservedGeneralPurposeRegisters[0]
	loc = s.pushRuntimeValueLocationOnRegister(tmpReg, runtimeValueTypeI64)
	require.Equal(t, uint64(2), s.sp)
	require.Equal(t, uint64(1), loc.stackPointer)
	require.Equal(t, tmpReg, loc.register)
	require.Equal(t, loc.valueType, runtimeValueTypeI64)
	// markRegisterUsed.
	tmpReg2 := unreservedGeneralPurposeRegisters[1]
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
		s.pushRuntimeValueLocationOnStack()
	}
	for i := 0; i < 1000; i++ {
		s.pop()
	}
	require.Equal(t, uint64(1002), s.stackPointerCeil)
}

func TestRuntimeValueLocationStack_takeFreeRegister(t *testing.T) {
	s := newRuntimeValueLocationStack()
	// For int registers.
	r, ok := s.takeFreeRegister(registerTypeGeneralPurpose)
	require.True(t, ok)
	require.True(t, isGeneralPurposeRegister(r))
	// Mark all the int registers used.
	for _, r := range unreservedGeneralPurposeRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for int.
	_, ok = s.takeFreeRegister(registerTypeGeneralPurpose)
	require.False(t, ok)
	// But we still should be able to take float regs.
	r, ok = s.takeFreeRegister(registerTypeVector)
	require.True(t, ok)
	require.True(t, isVectorRegister(r))
	// Mark all the float registers used.
	for _, r := range unreservedVectorRegisters {
		s.markRegisterUsed(r)
	}
	// Now we cannot take free ones for floats.
	_, ok = s.takeFreeRegister(registerTypeVector)
	require.False(t, ok)
}

func TestRuntimeValueLocationStack_takeStealTargetFromUsedRegister(t *testing.T) {
	s := newRuntimeValueLocationStack()
	intReg := unreservedGeneralPurposeRegisters[0]
	intLocation := &runtimeValueLocation{register: intReg}
	floatReg := unreservedVectorRegisters[0]
	floatLocation := &runtimeValueLocation{register: floatReg}
	s.push(intLocation)
	s.push(floatLocation)
	// Take for float.
	target, ok := s.takeStealTargetFromUsedRegister(registerTypeVector)
	require.True(t, ok)
	require.Equal(t, floatLocation, target)
	// Take for ints.
	target, ok = s.takeStealTargetFromUsedRegister(registerTypeGeneralPurpose)
	require.True(t, ok)
	require.Equal(t, intLocation, target)
	// Pop float value.
	popped := s.pop()
	require.Equal(t, floatLocation, popped)
	// Now we cannot find the steal target.
	target, ok = s.takeStealTargetFromUsedRegister(registerTypeVector)
	require.False(t, ok)
	require.Nil(t, target)
	// Pop int value.
	popped = s.pop()
	require.Equal(t, intLocation, popped)
	// Now we cannot find the steal target.
	target, ok = s.takeStealTargetFromUsedRegister(registerTypeGeneralPurpose)
	require.False(t, ok)
	require.Nil(t, target)
}
