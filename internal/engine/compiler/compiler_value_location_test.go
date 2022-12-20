package compiler

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
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
	require.Equal(t, s.unreservedGeneralPurposeRegisters, cloned.unreservedGeneralPurposeRegisters)
	require.Equal(t, s.unreservedVectorRegisters, cloned.unreservedVectorRegisters)
	require.Equal(t, len(s.stack), len(cloned.stack))
	require.Equal(t, s.sp, cloned.sp)
	for i := 0; i < int(s.sp); i++ {
		actual, exp := &s.stack[i], &cloned.stack[i]
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
	floatReg := unreservedVectorRegisters[0]
	intLocation := s.push(intReg, asm.ConditionalRegisterStateUnset)
	floatLocation := s.push(floatReg, asm.ConditionalRegisterStateUnset)
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

func TestRuntimeValueLocationStack_setupInitialStack(t *testing.T) {
	const f32 = wasm.ValueTypeF32
	tests := []struct {
		name       string
		sig        *wasm.FunctionType
		expectedSP uint64
	}{
		{
			name:       "no params / no results",
			sig:        &wasm.FunctionType{},
			expectedSP: callFrameDataSizeInUint64,
		},
		{
			name: "no results",
			sig: &wasm.FunctionType{
				Params:           []wasm.ValueType{f32, f32},
				ParamNumInUint64: 2,
			},
			expectedSP: callFrameDataSizeInUint64 + 2,
		},
		{
			name: "no params",
			sig: &wasm.FunctionType{
				Results:           []wasm.ValueType{f32, f32},
				ResultNumInUint64: 2,
			},
			expectedSP: callFrameDataSizeInUint64 + 2,
		},
		{
			name: "params == results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{f32, f32},
				ParamNumInUint64:  2,
				Results:           []wasm.ValueType{f32, f32},
				ResultNumInUint64: 2,
			},
			expectedSP: callFrameDataSizeInUint64 + 2,
		},
		{
			name: "params > results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{f32, f32, f32},
				ParamNumInUint64:  3,
				Results:           []wasm.ValueType{f32, f32},
				ResultNumInUint64: 2,
			},
			expectedSP: callFrameDataSizeInUint64 + 3,
		},
		{
			name: "params <  results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{f32},
				ParamNumInUint64:  1,
				Results:           []wasm.ValueType{f32, f32, f32},
				ResultNumInUint64: 3,
			},
			expectedSP: callFrameDataSizeInUint64 + 3,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := newRuntimeValueLocationStack()
			s.init(tc.sig)
			require.Equal(t, tc.expectedSP, s.sp)

			callFrameLocations := s.stack[s.sp-callFrameDataSizeInUint64 : s.sp]
			for _, loc := range callFrameLocations {
				require.Equal(t, runtimeValueTypeI64, loc.valueType)
			}
		})
	}
}

func TestRuntimeValueLocation_pushCallFrame(t *testing.T) {
	for _, sig := range []*wasm.FunctionType{
		{ParamNumInUint64: 0, ResultNumInUint64: 1},
		{ParamNumInUint64: 1, ResultNumInUint64: 0},
		{ParamNumInUint64: 1, ResultNumInUint64: 1},
		{ParamNumInUint64: 0, ResultNumInUint64: 2},
		{ParamNumInUint64: 2, ResultNumInUint64: 0},
		{ParamNumInUint64: 2, ResultNumInUint64: 3},
	} {
		sig := sig
		t.Run(sig.String(), func(t *testing.T) {
			s := newRuntimeValueLocationStack()
			// pushCallFrame assumes that the parameters are already pushed.
			s.sp += uint64(sig.ParamNumInUint64)

			retAddr, stackBasePointer, fn := s.pushCallFrame(sig)

			expOffset := uint64(callFrameOffset(sig))
			require.Equal(t, expOffset, retAddr.stackPointer)
			require.Equal(t, expOffset+1, stackBasePointer.stackPointer)
			require.Equal(t, expOffset+2, fn.stackPointer)
		})
	}
}
