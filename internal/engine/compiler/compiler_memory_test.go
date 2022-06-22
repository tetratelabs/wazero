package compiler

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileMemoryGrow(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, nil)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	err = compiler.compileMemoryGrow()
	require.NoError(t, err)

	// Emit arbitrary code after MemoryGrow returned so that we can verify
	// that the code can set the return address properly.
	const expValue uint32 = 100
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
	require.NoError(t, err)
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// After the initial exec, the code must exit with builtin function call status and funcaddress for memory grow.
	require.Equal(t, nativeCallStatusCodeCallBuiltInFunction, env.compilerStatus())
	require.Equal(t, builtinFunctionIndexMemoryGrow, env.builtinFunctionCallAddress())

	// Reenter from the return address.
	nativecall(
		env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.callEngine())),
		uintptr(unsafe.Pointer(env.module())),
	)

	// Check if the code successfully executed the code after builtin function call.
	require.Equal(t, expValue, env.stackTopAsUint32())
	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
}

func TestCompiler_compileMemorySize(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Emit memory.size instructions.
	err = compiler.compileMemorySize()
	require.NoError(t, err)
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
	require.Equal(t, uint32(defaultMemoryPageNumInTest), env.stackTopAsUint32())
}

func TestCompiler_compileLoad(t *testing.T) {
	// For testing. Arbitrary number is fine.
	loadTargetValue := uint64(0x12_34_56_78_9a_bc_ef_fe)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryArg{Offset: 361}
	offset := baseOffset + arg.Offset

	tests := []struct {
		name                string
		isFloatTarget       bool
		operationSetupFn    func(t *testing.T, compiler compilerImpl)
		loadedValueVerifyFn func(t *testing.T, loadedValueAsUint64 uint64)
	}{
		{
			name: "i32.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadTargetValue), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, loadTargetValue, loadedValueAsUint64)
			},
		},
		{
			name: "f32.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadTargetValue), uint32(loadedValueAsUint64))
			},
			isFloatTarget: true,
		},
		{
			name: "f64.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, loadTargetValue, loadedValueAsUint64)
			},
			isFloatTarget: true,
		},
		{
			name: "i32.load8s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int8(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load8u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(byte(loadedValueAsUint64)), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int8(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(byte(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i32.load16s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int16(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load16u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadedValueAsUint64), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int16(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint16(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i64.load32s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: true})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int32(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load32u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: false})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint32(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			binary.LittleEndian.PutUint64(env.memory()[offset:], loadTargetValue)

			// Before load operation, we must push the base offset value.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: baseOffset})
			require.NoError(t, err)

			tc.operationSetupFn(t, compiler)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
			loadedLocation := compiler.runtimeValueLocationStack().peek()
			require.True(t, loadedLocation.onRegister())
			if tc.isFloatTarget {
				require.Equal(t, registerTypeVector, loadedLocation.getRegisterType())
			} else {
				require.Equal(t, registerTypeGeneralPurpose, loadedLocation.getRegisterType())
			}
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Verify the loaded value.
			require.Equal(t, uint64(1), env.stackPointer())
			tc.loadedValueVerifyFn(t, env.stackTopAsUint64())
		})
	}
}

func TestCompiler_compileStore(t *testing.T) {
	// For testing. Arbitrary number is fine.
	storeTargetValue := uint64(math.MaxUint64)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryArg{Offset: 361}
	offset := arg.Offset + baseOffset

	tests := []struct {
		name                string
		isFloatTarget       bool
		targetSizeInBytes   uint32
		operationSetupFn    func(t *testing.T, compiler compilerImpl)
		storedValueVerifyFn func(t *testing.T, mem []byte)
	}{
		{
			name:              "i32.store",
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
		{
			name:              "f32.store",
			isFloatTarget:     true,
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
		{
			name:              "i64.store",
			targetSizeInBytes: 64 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, storeTargetValue, binary.LittleEndian.Uint64(mem[offset:]))
			},
		},
		{
			name:              "f64.store",
			isFloatTarget:     true,
			targetSizeInBytes: 64 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, storeTargetValue, binary.LittleEndian.Uint64(mem[offset:]))
			},
		},
		{
			name:              "store8",
			targetSizeInBytes: 1,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore8(&wazeroir.OperationStore8{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, byte(storeTargetValue), mem[offset])
			},
		},
		{
			name:              "store16",
			targetSizeInBytes: 16 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore16(&wazeroir.OperationStore16{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint16(storeTargetValue), binary.LittleEndian.Uint16(mem[offset:]))
			},
		},
		{
			name:              "store32",
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore32(&wazeroir.OperationStore32{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before store operations, we must push the base offset, and the store target values.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: baseOffset})
			require.NoError(t, err)
			if tc.isFloatTarget {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(storeTargetValue)})
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: storeTargetValue})
			}
			require.NoError(t, err)

			tc.operationSetupFn(t, compiler)

			// At this point, no registers must be in use, and no values on the stack since we consumed two values.
			require.Zero(t, len(compiler.runtimeValueLocationStack().usedRegisters))
			require.Equal(t, uint64(0), compiler.runtimeValueLocationStack().sp)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Set the value on the left and right neighboring memoryregion,
			// so that we can verify the operation doesn't affect there.
			ceil := offset + tc.targetSizeInBytes
			mem := env.memory()
			expectedNeighbor8Bytes := uint64(0x12_34_56_78_9a_bc_ef_fe)
			binary.LittleEndian.PutUint64(mem[offset-8:offset], expectedNeighbor8Bytes)
			binary.LittleEndian.PutUint64(mem[ceil:ceil+8], expectedNeighbor8Bytes)

			// Run code.
			env.exec(code)

			tc.storedValueVerifyFn(t, mem)

			// The neighboring bytes must be intact.
			require.Equal(t, expectedNeighbor8Bytes, binary.LittleEndian.Uint64(mem[offset-8:offset]))
			require.Equal(t, expectedNeighbor8Bytes, binary.LittleEndian.Uint64(mem[ceil:ceil+8]))
		})
	}
}

func TestCompiler_MemoryOutOfBounds(t *testing.T) {
	bases := []uint32{0, 1 << 5, 1 << 9, 1 << 10, 1 << 15, math.MaxUint32 - 1, math.MaxUint32}
	offsets := []uint32{0,
		1 << 10, 1 << 31,
		defaultMemoryPageNumInTest*wasm.MemoryPageSize - 1, defaultMemoryPageNumInTest * wasm.MemoryPageSize,
		math.MaxInt32 - 1, math.MaxInt32 - 2, math.MaxInt32 - 3, math.MaxInt32 - 4,
		math.MaxInt32 - 5, math.MaxInt32 - 8, math.MaxInt32 - 9, math.MaxInt32, math.MaxUint32,
	}
	targetSizeInBytes := []int64{1, 2, 4, 8}
	for _, base := range bases {
		base := base
		for _, offset := range offsets {
			offset := offset
			for _, targetSizeInByte := range targetSizeInBytes {
				targetSizeInByte := targetSizeInByte
				t.Run(fmt.Sprintf("base=%d,offset=%d,targetSizeInBytes=%d", base, offset, targetSizeInByte), func(t *testing.T) {
					env := newCompilerEnvironment()
					compiler := env.requireNewCompiler(t, newCompiler, nil)

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					arg := &wazeroir.MemoryArg{Offset: offset}

					switch targetSizeInByte {
					case 1:
						err = compiler.compileLoad8(&wazeroir.OperationLoad8{Type: wazeroir.SignedInt32, Arg: arg})
					case 2:
						err = compiler.compileLoad16(&wazeroir.OperationLoad16{Type: wazeroir.SignedInt32, Arg: arg})
					case 4:
						err = compiler.compileLoad32(&wazeroir.OperationLoad32{Signed: false, Arg: arg})
					case 8:
						err = compiler.compileLoad(&wazeroir.OperationLoad{Type: wazeroir.UnsignedTypeF64, Arg: arg})
					default:
						t.Fail()
					}

					require.NoError(t, err)

					require.NoError(t, compiler.compileReturnFunction())

					// Generate the code under test and run.
					code, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					mem := env.memory()
					if ceil := int64(base) + int64(offset) + int64(targetSizeInByte); int64(len(mem)) < ceil {
						// If the targe memory region's ceil exceeds the length of memory, we must exit the function
						// with nativeCallStatusCodeMemoryOutOfBounds status code.
						require.Equal(t, nativeCallStatusCodeMemoryOutOfBounds, env.compilerStatus())
					}
				})
			}
		}
	}
}
