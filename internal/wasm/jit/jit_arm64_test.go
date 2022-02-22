//go:build arm64
// +build arm64

package jit

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func requireAddLabel(t *testing.T, compiler *arm64Compiler, label *wazeroir.Label) {
	// We set a value location stack so that the label must not be skipped.
	compiler.label(label.String()).initialStack = newValueLocationStack()
	skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
	require.False(t, skip)
}

func requirePushTwoInt32Consts(t *testing.T, x1, x2 uint32, compiler *arm64Compiler) {
	err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2})
	require.NoError(t, err)
}

func requirePushTwoFloat32Consts(t *testing.T, x1, x2 float32, compiler *arm64Compiler) {
	err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x2})
	require.NoError(t, err)
}

func (j *jitEnv) requireNewCompiler(t *testing.T) *arm64Compiler {
	cmp, done, err := newCompiler(&wasm.FunctionInstance{
		ModuleInstance: j.moduleInstance,
		FunctionKind:   wasm.FunctionKindWasm,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(done)

	ret, ok := cmp.(*arm64Compiler)
	require.True(t, ok)
	ret.labels = make(map[string]*labelInfo)
	ret.ir = &wazeroir.CompilationResult{}
	return ret
}

func TestArchContextOffsetInEngine(t *testing.T) {
	var eng engine
	require.Equal(t, int(unsafe.Offsetof(eng.jitCallReturnAddress)), engineArchContextJITCallReturnAddressOffset) // If this fails, we have to fix jit_arm64.s as well.
	require.Equal(t, int(unsafe.Offsetof(eng.minimum32BitSignedInt)), engineArchContextMinimum32BitSignedIntOffset)
	require.Equal(t, int(unsafe.Offsetof(eng.minimum64BitSignedInt)), engineArchContextMinimum64BitSignedIntOffset)
}

func TestArm64Compiler_returnFunction(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		env := newJITEnvironment()

		// Build code.
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		compiler.compileReturnFunction()

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)

		// JIT status on engine must be returned.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		// Plus, the call frame stack pointer must be zero after return.
		require.Equal(t, uint64(0), env.callFrameStackPointer())
	})
	t.Run("deep call stack", func(t *testing.T) {
		env := newJITEnvironment()
		engine := env.engine()

		// Push the call frames.
		const callFrameNums = 10
		stackPointerToExpectedValue := map[uint64]uint32{}
		for funcaddr := wasm.FunctionAddress(0); funcaddr < callFrameNums; funcaddr++ {
			// We have to do compilation in a separate subtest since each compilation takes
			// the mutext lock and must release on the cleanup of each subtest.
			// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
			t.Run(fmt.Sprintf("compiling existing callframe %d", funcaddr), func(t *testing.T) {
				// Each function pushes its funcaddr and soon returns.
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Push its funcaddr.
				expValue := uint32(funcaddr)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
				require.NoError(t, err)

				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Compiles and adds to the engine.
				compiledFunction := &compiledFunction{codeSegment: code, codeInitialAddress: uintptr(unsafe.Pointer(&code[0]))}
				engine.addCompiledFunction(funcaddr, compiledFunction)

				// Pushes the frame whose return address equals the beginning of the function just compiled.
				frame := callFrame{
					// Set the return address to the beginning of the function so that we can execute the constI32 above.
					returnAddress: compiledFunction.codeInitialAddress,
					// Note: return stack base pointer is set to funcaddr*10 and this is where the const should be pushed.
					returnStackBasePointer: uint64(funcaddr) * 10,
					compiledFunction:       compiledFunction,
				}
				engine.callFrameStack[engine.globalContext.callFrameStackPointer] = frame
				engine.globalContext.callFrameStackPointer++
				stackPointerToExpectedValue[frame.returnStackBasePointer] = expValue
			})
		}

		require.Equal(t, uint64(callFrameNums), env.callFrameStackPointer())

		// Run code from the top frame.
		env.exec(engine.callFrameTop().compiledFunction.codeSegment)

		// Check the exit status and the values on stack.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		for pos, exp := range stackPointerToExpectedValue {
			require.Equal(t, exp, uint32(env.stack()[pos]))
		}
	})
}

func TestArm64Compiler_exit(t *testing.T) {
	for _, s := range []jitCallStatusCode{
		jitCallStatusCodeReturned,
		jitCallStatusCodeCallHostFunction,
		jitCallStatusCodeCallBuiltInFunction,
		jitCallStatusCodeUnreachable,
	} {
		t.Run(s.String(), func(t *testing.T) {

			env := newJITEnvironment()

			// Build code.
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()

			expStackPointer := uint64(100)
			compiler.locationStack.sp = expStackPointer
			require.NoError(t, err)
			compiler.compileExitFromNativeCode(s)

			// Compile and execute the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// JIT status on engine must be updated.
			require.Equal(t, s, env.jitStatus())

			// Stack pointer must be written on engine.stackPointer on return.
			require.Equal(t, expStackPointer, env.stackPointer())
		})
	}
}

func TestArm64Compiler_compileConsts(t *testing.T) {
	for _, op := range []wazeroir.OperationKind{
		wazeroir.OperationKindConstI32,
		wazeroir.OperationKindConstI64,
		wazeroir.OperationKindConstF32,
		wazeroir.OperationKindConstF64,
	} {
		op := op
		t.Run(op.String(), func(t *testing.T) {
			for _, val := range []uint64{
				0x0, 0x1, 0x1111000, 1 << 16, 1 << 21, 1 << 27, 1 << 32, 1<<32 + 1, 1 << 53,
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
				math.Float64bits(math.NaN()),
				math.MaxUint32,
				math.MaxInt32,
				math.MaxUint64,
				math.MaxInt64,
				uint64(math.Float32bits(float32(math.Inf(1)))),
				uint64(math.Float32bits(float32(math.Inf(-1)))),
				uint64(math.Float32bits(float32(math.NaN()))),
			} {
				t.Run(fmt.Sprintf("0x%x", val), func(t *testing.T) {
					env := newJITEnvironment()

					// Build code.
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					switch op {
					case wazeroir.OperationKindConstI32:
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(val)})
					case wazeroir.OperationKindConstI64:
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
					case wazeroir.OperationKindConstF32:
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(val))})
					case wazeroir.OperationKindConstF64:
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
					}
					require.NoError(t, err)

					// After compiling const operations, we must see the register allocated value on the top of value.
					loc := compiler.locationStack.peek()
					require.True(t, loc.onRegister())

					// Release the register allocated value to the memory stack so that we can see the value after exiting.
					compiler.compileReleaseRegisterToStack(loc)
					compiler.compileReturnFunction()

					// Generate the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					// Run native code.
					env.exec(code)

					// JIT status on engine must be returned.
					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer())

					switch op {
					case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstF32:
						require.Equal(t, uint32(val), env.stackTopAsUint32())
					case wazeroir.OperationKindConstI64, wazeroir.OperationKindConstF64:
						require.Equal(t, val, env.stackTopAsUint64())
					}
				})
			}
		})
	}
}

func TestArm64Compiler_releaseRegisterToStack(t *testing.T) {
	const val = 10000
	for _, tc := range []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()

			// Build code.
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			compiler.locationStack.sp = tc.stackPointer
			compiler.locationStack.stack = make([]*valueLocation, tc.stackPointer)
			// Peek must be non-nil. Otherwise, compileConst* would fail.
			compiler.locationStack.stack[compiler.locationStack.sp-1] = &valueLocation{}

			if tc.isFloat {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
			}
			require.NoError(t, err)

			// Release the register allocated value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(compiler.locationStack.peek())
			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack.
			env.engine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.exec(code)

			// JIT status on engine must be returned and stack pointer must end up the specified one.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val), env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val), env.stackTopAsUint64())
			}
		})
	}
}

func TestArm64Compiler_compileLoadValueOnStackToRegister(t *testing.T) {
	const val = 123
	for _, tc := range []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()

			// Build code.
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			compiler.locationStack.sp = tc.stackPointer
			compiler.locationStack.stack = make([]*valueLocation, tc.stackPointer)

			// Record that that top value is on top.
			require.Len(t, compiler.locationStack.usedRegisters, 0)
			loc := compiler.locationStack.pushValueLocationOnStack()
			if tc.isFloat {
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			} else {
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			}
			// At this point the value must be recorded as being on stack.
			require.True(t, loc.onStack())

			// Release the stack-allocated value to register.
			compiler.compileLoadValueOnStackToRegister(loc)
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			require.True(t, loc.onRegister())

			// To verify the behavior, increment the value on the register.
			if tc.isFloat {
				// For float, we cannot add consts, so load the constant first.
				err = compiler.compileFloatConstant(false, math.Float64bits(1))
				require.NoError(t, err)
				// Then, do the increment.
				compiler.compileRegisterToRegisterInstruction(arm64.AFADDD, compiler.locationStack.peek().register, loc.register)
				// Delete the loaded const.
				compiler.locationStack.pop()
			} else {
				compiler.compileConstToRegisterInstruction(arm64.AADD, 1, loc.register)
			}

			// Release the value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(loc)
			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack, and place the original value.
			env.engine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.stack()[tc.stackPointer] = val
			env.exec(code)

			// JIT status on engine must be returned and stack pointer must end up the specified one.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val)+1, env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val)+1, env.stackTopAsUint64())
			}
		})
	}
}

// TODO: break this up somehow so that the test name is more readable
func TestArm64Compiler_compile_Le_Lt_Gt_Ge_Eq_Eqz_Ne(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindEq,
		wazeroir.OperationKindEqz,
		wazeroir.OperationKindNe,
		wazeroir.OperationKindLe,
		wazeroir.OperationKindLt,
		wazeroir.OperationKindGe,
		wazeroir.OperationKindGt,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, signedType := range []wazeroir.SignedType{
				wazeroir.SignedTypeUint32,
				wazeroir.SignedTypeUint64,
				wazeroir.SignedTypeInt32,
				wazeroir.SignedTypeInt64,
				wazeroir.SignedTypeFloat32,
				wazeroir.SignedTypeFloat64,
			} {
				signedType := signedType
				t.Run(signedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{1, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
						{math.Float64bits(math.MaxFloat32), 1},
						{math.Float64bits(math.SmallestNonzeroFloat32), 1},
						{math.Float64bits(math.MaxFloat64), 1},
						{math.Float64bits(math.SmallestNonzeroFloat64), 1},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 0},
						{math.Float64bits(math.Inf(-1)), 0},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						isEqz := kind == wazeroir.OperationKindEqz
						if isEqz && (signedType == wazeroir.SignedTypeFloat32 || signedType == wazeroir.SignedTypeFloat64) {
							// Eqz isn't defined for float.
							t.Skip()
						}
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							if isEqz {
								// Eqz only needs one value, so pop the top one (x2).
								compiler.locationStack.pop()
								require.Equal(t, uint64(1), compiler.locationStack.sp)
							} else {
								// At this point, two values exist for comparison.
								require.Equal(t, uint64(2), compiler.locationStack.sp)
							}

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindLe:
								err = compiler.compileLe(&wazeroir.OperationLe{Type: signedType})
							case wazeroir.OperationKindLt:
								err = compiler.compileLt(&wazeroir.OperationLt{Type: signedType})
							case wazeroir.OperationKindGe:
								err = compiler.compileGe(&wazeroir.OperationGe{Type: signedType})
							case wazeroir.OperationKindGt:
								err = compiler.compileGt(&wazeroir.OperationGt{Type: signedType})
							case wazeroir.OperationKindEq:
								// Eq uses UnsignedType instead, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI64})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF32})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF64})
								}
							case wazeroir.OperationKindNe:
								// Ne uses UnsignedType, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI64})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF32})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF64})
								}
							case wazeroir.OperationKindEqz:
								// Eqz uses UnsignedInt, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt64})
								}
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.locationStack.sp)
							resultLocation := compiler.locationStack.peek()
							// Plus the result must be located on a conditional register.
							require.True(t, resultLocation.onConditionalRegister())

							// Move the conditional register value to a general purpose register to verify the value.
							compiler.compileLoadConditionalRegisterToGeneralPurposeRegister(resultLocation)
							require.True(t, resultLocation.onRegister())

							compiler.compileReturnFunction()

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// There should only be one value on the stack
							require.Equal(t, uint64(1), env.stackPointer())

							actual := env.stackTopAsUint32() == 1

							switch kind {
							case wazeroir.OperationKindLe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) <= int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) <= uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) <= int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 <= x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) <= math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) <= math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindLt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) < int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) < uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) < int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 < x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) < math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) < math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindGe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) >= int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) >= uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) >= int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 >= x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) >= math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) >= math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindGt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) > int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) > uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) > int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 > x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) > math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) > math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindEq:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) == uint32(x2), actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 == x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) == math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) == math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindNe:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) != uint32(x2), actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 != x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) != math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) != math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindEqz:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) == 0, actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 == 0, actual)
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compile_Add_Sub_Mul(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindAdd,
		wazeroir.OperationKindSub,
		wazeroir.OperationKindMul,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, unsignedType := range []wazeroir.UnsignedType{
				wazeroir.UnsignedTypeI32,
				wazeroir.UnsignedTypeI64,
				wazeroir.UnsignedTypeF32,
				wazeroir.UnsignedTypeF64,
			} {
				unsignedType := unsignedType
				t.Run(unsignedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.UnsignedTypeI64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.UnsignedTypeF32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.UnsignedTypeF64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist.
							require.Equal(t, uint64(2), compiler.locationStack.sp)

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindAdd:
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: unsignedType})
							case wazeroir.OperationKindSub:
								err = compiler.compileSub(&wazeroir.OperationSub{Type: unsignedType})
							case wazeroir.OperationKindMul:
								err = compiler.compileMul(&wazeroir.OperationMul{Type: unsignedType})
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.locationStack.sp)
							resultLocation := compiler.locationStack.peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onRegister())
							// Also, the result must have an appropriate register type.
							if unsignedType == wazeroir.UnsignedTypeF32 || unsignedType == wazeroir.UnsignedTypeF64 {
								require.Equal(t, generalPurposeRegisterTypeFloat, resultLocation.regType)
							} else {
								require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)
							}

							// Release the value to the memory stack again to verify the operation.
							compiler.compileReleaseRegisterToStack(resultLocation)
							compiler.compileReturnFunction()

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Check the stack.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindAdd:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)+uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1+x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) + math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) + math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindSub:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)-uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1-x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) - math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) - math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindMul:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)*uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1*x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) * math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) * math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compile_And_Or_Xor_Shl_Rotr(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindAnd,
		wazeroir.OperationKindOr,
		wazeroir.OperationKindXor,
		wazeroir.OperationKindShl,
		wazeroir.OperationKindRotl,
		wazeroir.OperationKindRotr,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, unsignedInt := range []wazeroir.UnsignedInt{
				wazeroir.UnsignedInt32,
				wazeroir.UnsignedInt64,
			} {
				unsignedInt := unsignedInt
				t.Run(unsignedInt.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {0, 1}, {1, 0}, {1, 1},
						{1 << 31, 1}, {1, 1 << 31}, {1 << 31, 1 << 31},
						{1 << 63, 1}, {1, 1 << 63}, {1 << 63, 1 << 63},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.UnsignedInt64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist.
							require.Equal(t, uint64(2), compiler.locationStack.sp)

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindAnd:
								err = compiler.compileAnd(&wazeroir.OperationAnd{Type: unsignedInt})
							case wazeroir.OperationKindOr:
								err = compiler.compileOr(&wazeroir.OperationOr{Type: unsignedInt})
							case wazeroir.OperationKindXor:
								err = compiler.compileXor(&wazeroir.OperationXor{Type: unsignedInt})
							case wazeroir.OperationKindShl:
								err = compiler.compileShl(&wazeroir.OperationShl{Type: unsignedInt})
							case wazeroir.OperationKindRotl:
								err = compiler.compileRotl(&wazeroir.OperationRotl{Type: unsignedInt})
							case wazeroir.OperationKindRotr:
								err = compiler.compileRotr(&wazeroir.OperationRotr{Type: unsignedInt})
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.locationStack.sp)
							resultLocation := compiler.locationStack.peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onRegister())
							// Also, the result must have an appropriate register type.
							require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)

							// Release the value to the memory stack again to verify the operation.
							compiler.compileReleaseRegisterToStack(resultLocation)
							compiler.compileReturnFunction()

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Check the stack.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindAnd:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)&uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1&x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindOr:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)|uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1|x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindXor:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)^uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1^x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindShl:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)<<uint32(x2%32), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1<<(x2%64), env.stackTopAsUint64())
								}
							case wazeroir.OperationKindRotl:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, bits.RotateLeft32(uint32(x1), int(x2)), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, bits.RotateLeft64(x1, int(x2)), env.stackTopAsUint64())
								}
							case wazeroir.OperationKindRotr:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, bits.RotateLeft32(uint32(x1), -int(x2)), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, bits.RotateLeft64(x1, -int(x2)), env.stackTopAsUint64())
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compileShr(t *testing.T) {
	kind := wazeroir.OperationKindShr
	t.Run(kind.String(), func(t *testing.T) {
		for _, signedInt := range []wazeroir.SignedInt{
			wazeroir.SignedInt32,
			wazeroir.SignedInt64,
			wazeroir.SignedUint32,
			wazeroir.SignedUint64,
		} {
			signedInt := signedInt
			t.Run(signedInt.String(), func(t *testing.T) {
				for _, values := range [][2]uint64{
					{0, 0}, {0, 1}, {1, 0}, {1, 1},
					{1 << 31, 1}, {1, 1 << 31}, {1 << 31, 1 << 31},
					{1 << 63, 1}, {1, 1 << 63}, {1 << 63, 1 << 63},
				} {
					x1, x2 := values[0], values[1]
					t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Emit consts operands.
						for _, v := range []uint64{x1, x2} {
							switch signedInt {
							case wazeroir.SignedInt32:
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
							case wazeroir.SignedInt64:
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							case wazeroir.SignedUint32:
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
							case wazeroir.SignedUint64:
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)
						}

						// At this point, two values exist.
						require.Equal(t, uint64(2), compiler.locationStack.sp)

						// Emit the operation.
						err = compiler.compileShr(&wazeroir.OperationShr{Type: signedInt})
						require.NoError(t, err)

						// We consumed two values, but push the result back.
						require.Equal(t, uint64(1), compiler.locationStack.sp)
						resultLocation := compiler.locationStack.peek()
						// Plus the result must be located on a register.
						require.True(t, resultLocation.onRegister())
						// Also, the result must have an appropriate register type.
						require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)

						// Release the value to the memory stack again to verify the operation.
						compiler.compileReleaseRegisterToStack(resultLocation)
						compiler.compileReturnFunction()

						// Compile and execute the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())

						switch signedInt {
						case wazeroir.SignedInt32:
							require.Equal(t, int32(x1)>>(uint32(x2)%32), env.stackTopAsInt32())
						case wazeroir.SignedInt64:
							require.Equal(t, int64(x1)>>(x2%64), env.stackTopAsInt64())
						case wazeroir.SignedUint32:
							require.Equal(t, uint32(x1)>>(uint32(x2)%32), env.stackTopAsUint32())
						case wazeroir.SignedUint64:
							require.Equal(t, x1>>(x2%64), env.stackTopAsUint64())
						}
					})
				}
			})
		}
	})
}

func TestArm64Compiler_compilePick(t *testing.T) {
	const pickTargetValue uint64 = 12345
	op := &wazeroir.OperationPick{Depth: 1}

	for _, tc := range []struct {
		name                                      string
		pickTargetSetupFunc                       func(compiler *arm64Compiler, eng *engine) error
		isPickTargetFloat, isPickTargetOnRegister bool
	}{
		{
			name: "float on register",
			pickTargetSetupFunc: func(compiler *arm64Compiler, eng *engine) error {
				return compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(pickTargetValue)})
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: true,
		},
		{
			name: "int on register",
			pickTargetSetupFunc: func(compiler *arm64Compiler, eng *engine) error {
				return compiler.compileConstI64(&wazeroir.OperationConstI64{Value: pickTargetValue})
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: true,
		},
		{
			name: "float on stack",
			pickTargetSetupFunc: func(compiler *arm64Compiler, eng *engine) error {
				pickTargetLocation := compiler.locationStack.pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeFloat)
				eng.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: false,
		},
		{
			name: "int on stack",
			pickTargetSetupFunc: func(compiler *arm64Compiler, eng *engine) error {
				pickTargetLocation := compiler.locationStack.pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeInt)
				eng.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the stack before picking.
			err = tc.pickTargetSetupFunc(compiler, env.engine())
			require.NoError(t, err)
			pickTargetLocation := compiler.locationStack.peek()

			// Push the unused median value.
			_ = compiler.locationStack.pushValueLocationOnStack()
			require.Equal(t, uint64(2), compiler.locationStack.sp)

			// Now ready to compile Pick operation.
			err = compiler.compilePick(op)
			require.NoError(t, err)
			require.Equal(t, uint64(3), compiler.locationStack.sp)

			pickedLocation := compiler.locationStack.peek()
			require.True(t, pickedLocation.onRegister())
			require.Equal(t, pickTargetLocation.registerType(), pickedLocation.registerType())

			// Release the value to the memory stack again to verify the operation, and then return.
			compiler.compileReleaseRegisterToStack(pickedLocation)
			if tc.isPickTargetOnRegister {
				compiler.compileReleaseRegisterToStack(pickTargetLocation)
			}
			compiler.compileReturnFunction()

			// Compile and execute the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the returned status and stack pointer.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, uint64(3), env.stackPointer())

			// Verify the top value is the picked one and the pick target's value stays the same.
			if tc.isPickTargetFloat {
				require.Equal(t, math.Float64frombits(pickTargetValue), env.stackTopAsFloat64())
				require.Equal(t, math.Float64frombits(pickTargetValue), math.Float64frombits(env.stack()[pickTargetLocation.stackPointer]))
			} else {
				require.Equal(t, pickTargetValue, env.stackTopAsUint64())
				require.Equal(t, pickTargetValue, env.stack()[pickTargetLocation.stackPointer])
			}
		})
	}
}

func TestArm64Compiler_compileDrop(t *testing.T) {
	t.Run("range nil", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		liveNum := 10
		for i := 0; i < liveNum; i++ {
			compiler.locationStack.pushValueLocationOnStack()
		}
		require.Equal(t, uint64(liveNum), compiler.locationStack.sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: nil})
		require.NoError(t, err)

		// After the nil range drop, the stack must remain the same.
		require.Equal(t, uint64(liveNum), compiler.locationStack.sp)

		compiler.compileReturnFunction()

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
	t.Run("start top", func(t *testing.T) {
		r := &wazeroir.InclusiveRange{Start: 0, End: 2}
		dropTargetNum := r.End - r.Start + 1 // +1 as the range is inclusive!
		liveNum := 5

		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		const expectedTopLiveValue = 100
		for i := 0; i < liveNum+dropTargetNum; i++ {
			if i == liveNum-1 {
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
				require.NoError(t, err)
			} else {
				compiler.locationStack.pushValueLocationOnStack()
			}
		}
		require.Equal(t, uint64(liveNum+dropTargetNum), compiler.locationStack.sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveNum), compiler.locationStack.sp)
		// Plus, the top value must stay on a register.
		top := compiler.locationStack.peek()
		require.True(t, top.onRegister())
		// Release the top value after drop so that we can verify the cpu itself is not mainpulated.
		compiler.compileReleaseRegisterToStack(top)

		compiler.compileReturnFunction()

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(5), env.stackPointer())
		require.Equal(t, uint64(expectedTopLiveValue), env.stackTopAsUint64())
	})

	t.Run("start from middle", func(t *testing.T) {
		r := &wazeroir.InclusiveRange{Start: 2, End: 3}
		liveAboveDropStartNum := 3
		dropTargetNum := r.End - r.Start + 1 // +1 as the range is inclusive!
		liveBelowDropEndNum := 5
		total := liveAboveDropStartNum + dropTargetNum + liveBelowDropEndNum
		liveTotal := liveAboveDropStartNum + liveBelowDropEndNum

		env := newJITEnvironment()
		eng := env.engine()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents except the top on stack
		for i := 0; i < total-1; i++ {
			loc := compiler.locationStack.pushValueLocationOnStack()
			eng.valueStack[loc.stackPointer] = uint64(i) // Put the initial value.
		}

		// Place the top value.
		const expectedTopLiveValue = 100
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
		require.NoError(t, err)

		require.Equal(t, uint64(total), compiler.locationStack.sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveTotal), compiler.locationStack.sp)
		// Plus, the top value must stay on a register.
		require.True(t, compiler.locationStack.peek().onRegister())

		// Release all register values so that we can verify the register allocated values.
		err = compiler.compileReleaseAllRegistersToStack()
		require.NoError(t, err)
		compiler.compileReturnFunction()

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(liveTotal), env.stackPointer())

		stack := env.stack()[:env.stackPointer()]
		for i, val := range stack {
			if i <= liveBelowDropEndNum {
				require.Equal(t, uint64(i), val)
			} else if i == liveTotal-1 {
				require.Equal(t, uint64(expectedTopLiveValue), val)
			} else {
				require.Equal(t, uint64(i+dropTargetNum), val)
			}
		}
	})
}

func TestArm64Compiler_compileLabel(t *testing.T) {
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	labelKey := label.String()
	for _, expectSkip := range []bool{false, true} {
		expectSkip := expectSkip
		t.Run(fmt.Sprintf("expect skip=%v", expectSkip), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)

			var callBackCalled bool
			compiler.labels[labelKey] = &labelInfo{
				labelBeginningCallbacks: []func(*obj.Prog){func(p *obj.Prog) { callBackCalled = true }},
			}

			if expectSkip {
				// If the initial stack is not set, compileLabel must return skip=true.
				compiler.labels[labelKey].initialStack = nil
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.True(t, actual)
				// Also, callback must not be called.
				require.False(t, callBackCalled)
			} else {
				// If the initial stack is not set, compileLabel must return skip=false.
				compiler.labels[labelKey].initialStack = newValueLocationStack()
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.False(t, actual)
				// Also, callback must be called.
				require.True(t, callBackCalled)
			}
		})
	}
}

func TestArm64Compiler_compileBr(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Branch into nil label is interpreted as return. See BranchTarget.IsReturnTarget
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: nil}})
		require.NoError(t, err)

		// Compile and execute the code under test.
		// Note: we don't invoke "compiler.return()" as the code emitted by compilerBr is enough to exit.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
	t.Run("backward br", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		// Emit code for the backward label.
		backwardLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		requireAddLabel(t, compiler, backwardLabel)
		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

		// Now emit the body. First we add NOP so that we can execute code after the target label.
		nop := compiler.compileNOP()

		err := compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: backwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with unreachable status.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		// .backwardLabel:
		//    exit jitCallStatusCodeReturned
		//    nop
		//    ... code from compilePreamble()
		//    br .backwardLabel
		//    exit jitCallStatusCodeUnreachable
		//
		// Therefore, if we start executing from nop, we must end up exiting jitCallStatusCodeReturned.
		env.exec(code[nop.Pc:])
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
	t.Run("forward br", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Emit the forward br, meaning that handle Br instruction where the target label hasn't been compiled yet.
		forwardLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: forwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with Unreachable status.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)

		// Emit code for the forward label where we emit the expectedValue and then exit.
		requireAddLabel(t, compiler, forwardLabel)
		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		//    ... code from compilePreamble()
		//    br .forwardLabel
		//    exit jitCallStatusCodeUnreachable
		// .forwardLabel:
		//    exit jitCallStatusCodeReturned
		//
		// Therefore, if we start executing from the top, we must end up exiting jitCallStatusCodeReturned.
		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestArm64Compiler_compileBrIf(t *testing.T) {
	unreachableStatus, thenLabelExitStatus, elseLabelExitStatus :=
		jitCallStatusCodeUnreachable, jitCallStatusCodeUnreachable+1, jitCallStatusCodeUnreachable+2
	thenBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}}}
	elseBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 2}}}

	for _, tc := range []struct {
		name      string
		setupFunc func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool)
	}{
		{
			name: "cond on register",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				val := uint32(1)
				if shoulGoElse {
					val = 0
				}
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: val})
				require.NoError(t, err)
			},
		},
		{
			name: "LS",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Le on unsigned integer produces the value on COND_LS register.
				err := compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "LE",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Le on signed integer produces the value on COND_LE register.
				err := compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "HS",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Ge on unsigned integer produces the value on COND_HS register.
				err := compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "GE",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Ge on signed integer produces the value on COND_GE register.
				err := compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "HI",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Gt on unsigned integer produces the value on COND_HI register.
				err := compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "GT",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Gt on signed integer produces the value on COND_GT register.
				err := compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "LO",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Lt on unsigned integer produces the value on COND_LO register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "LT",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Lt on signed integer produces the value on COND_LT register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "MI",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := float32(1), float32(2)
				if shoulGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoFloat32Consts(t, x1, x2, compiler)
				// Lt on floats produces the value on COND_MI register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat32})
				require.NoError(t, err)
			},
		},
		{
			name: "EQ",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(1)
				if shoulGoElse {
					x2++
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
		{
			name: "NE",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shoulGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shoulGoElse {
					x2 = x1
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, shouldGoToElse := range []bool{false, true} {
				shouldGoToElse := shouldGoToElse
				t.Run(fmt.Sprintf("should_goto_else=%v", shouldGoToElse), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					tc.setupFunc(t, compiler, shouldGoToElse)
					require.Equal(t, uint64(1), compiler.locationStack.sp)

					err = compiler.compileBrIf(&wazeroir.OperationBrIf{Then: thenBranchTarget, Else: elseBranchTarget})
					require.NoError(t, err)
					compiler.compileExitFromNativeCode(unreachableStatus)

					// Emit code for .then label.
					requireAddLabel(t, compiler, thenBranchTarget.Target.Label)
					compiler.compileExitFromNativeCode(thenLabelExitStatus)

					// Emit code for .else label.
					requireAddLabel(t, compiler, elseBranchTarget.Target.Label)
					compiler.compileExitFromNativeCode(elseLabelExitStatus)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					// The generated code looks like this:
					//
					//    ... code from compilePreamble()
					//    ... code from tc.setupFunc()
					//    br_if .then, .else
					//    exit $unreachableStatus
					// .then:
					//    exit $thenLabelExitStatus
					// .else:
					//    exit $elseLabelExitStatus
					//
					// Therefore, if we start executing from the top, we must end up exiting with an appropriate status.
					env.exec(code)
					require.NotEqual(t, unreachableStatus, env.jitStatus())
					if shouldGoToElse {
						require.Equal(t, elseLabelExitStatus, env.jitStatus())
					} else {
						require.Equal(t, thenLabelExitStatus, env.jitStatus())
					}
				})
			}
		})
	}
}

func TestArm64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("target instruction not found", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.compileReadInstructionAddress(obj.AJMP, reservedRegisterForTemporary)

		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

		// If generate the code without JMP after compileReadInstructionAddress,
		// the call back added must return error.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
		require.Contains(t, err.Error(), "target instruction not found")
	})
	t.Run("too large offset", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after RET.
		compiler.compileReadInstructionAddress(obj.ARET, reservedRegisterForTemporary)

		// Add many instruction between the target and compileReadInstructionAddress.
		for i := 0; i < 100; i++ {
			compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
		}

		ret := compiler.newProg()
		ret.As = obj.ARET
		ret.To.Type = obj.TYPE_REG
		ret.To.Reg = reservedRegisterForTemporary
		compiler.compileReturnFunction()

		// If generate the code with too many instruction between ADR and
		// the target, compile must fail.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
		require.Contains(t, err.Error(), "too large offset")
	})
	t.Run("ok", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		const addressReg = reservedRegisterForTemporary
		compiler.compileReadInstructionAddress(obj.ARET, addressReg)

		// Branch to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		compiler.compileUnconditionalBranchToAddressOnRegister(addressReg)

		// If we fail to branch, we reach here and exit with unreachable status,
		// so the assertion would fail.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the branch instruction above
		// must target here.
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestArm64Compiler_compileCall(t *testing.T) {
	for _, growCallFrameStack := range []bool{false, true} {
		growCallFrameStack := growCallFrameStack
		t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
			env := newJITEnvironment()
			engine := env.engine()
			expectedValue := uint32(0)

			if growCallFrameStack {
				env.setCallFrameStackPointer(engine.globalContext.callFrameStackLen - 1)
				env.setPreviousCallFrameStackPointer(engine.globalContext.callFrameStackLen - 1)
			}

			// Emit the call target function.
			const numCalls = 10
			targetFunctionType := &wasm.FunctionType{
				Params:  []wasm.ValueType{wasm.ValueTypeI32},
				Results: []wasm.ValueType{wasm.ValueTypeI32},
			}
			for i := 0; i < numCalls; i++ {
				// Each function takes one arguments, adds the value with 100 + i and returns the result.
				addTargetValue := uint32(100 + i)
				expectedValue += addTargetValue

				// We have to do compilation in a separate subtest since each compilation takes
				// the mutext lock and must release on the cleanup of each subtest.
				// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
				t.Run(fmt.Sprintf("compiling call target %d", i), func(t *testing.T) {
					compiler := env.requireNewCompiler(t)
					compiler.f = &wasm.FunctionInstance{
						FunctionKind:   wasm.FunctionKindWasm,
						FunctionType:   &wasm.TypeInstance{Type: targetFunctionType},
						ModuleInstance: &wasm.ModuleInstance{},
					}

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(addTargetValue)})
					require.NoError(t, err)
					err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
					require.NoError(t, err)
					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					addr := wasm.FunctionAddress(i)
					engine.addCompiledFunction(addr, &compiledFunction{
						codeSegment:        code,
						codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
					})
					env.moduleInstance.Functions = append(env.moduleInstance.Functions,
						&wasm.FunctionInstance{FunctionType: &wasm.TypeInstance{Type: targetFunctionType}, Address: addr})
				})
			}

			// Now we start building the caller's code.
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			const initialValue = 100
			expectedValue += initialValue
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0}) // Dummy value so the base pointer would be non-trivial for callees.
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: initialValue})
			require.NoError(t, err)

			// Call all the built functions.
			for i := 0; i < numCalls; i++ {
				err = compiler.compileCall(&wazeroir.OperationCall{FunctionIndex: uint32(i)})
				require.NoError(t, err)
			}

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			if growCallFrameStack {
				// If the call frame stack pointer equals the length of call frame stack length,
				// we have to call the builtin function to grow the slice.
				require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
				require.Equal(t, builtinFunctionAddressGrowCallFrameStack, env.functionCallAddress(), env.functionCallAddress())

				// Grow the callFrame stack, and exec again from the return address.
				env.engine().builtinFunctionGrowCallFrameStack()
				jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.engine())))
			}

			// Check status and returned values.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, uint64(2), env.stackPointer()) // Must be 2 (dummy value + the calculation results)
			require.Equal(t, uint64(0), env.stackBasePointer())
			require.Equal(t, expectedValue, env.stackTopAsUint32())
		})
	}
}

func TestArm64Compiler_compileCallIndirect(t *testing.T) {
	t.Run("out of bounds", func(t *testing.T) {
		env := newJITEnvironment()
		env.setTable(make([]wasm.TableElement, 10))
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex.
		compiler.f = &wasm.FunctionInstance{
			FunctionKind:   wasm.FunctionKindWasm,
			ModuleInstance: &wasm.ModuleInstance{Types: []*wasm.TypeInstance{{Type: &wasm.FunctionType{}}}},
		}

		// Place the offfset value.
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
		require.NoError(t, err)

		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequet code must be unreachable.
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("uninitialized", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		compiler.f = &wasm.FunctionInstance{
			ModuleInstance: &wasm.ModuleInstance{Types: []*wasm.TypeInstance{{Type: &wasm.FunctionType{}}}},
			FunctionKind:   wasm.FunctionKindWasm,
		}
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]wasm.TableElement, 10)
		env.setTable(table)
		table[0] = wasm.TableElement{FunctionTypeID: wasm.UninitializedTableElementTypeID}

		// Place the offset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)
		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequet code must be unreachable.
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("type not match", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		env.moduleInstance.Types = []*wasm.TypeInstance{{Type: &wasm.FunctionType{}, TypeID: 1000}}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]wasm.TableElement, 10)
		env.setTable(table)
		table[0] = wasm.TableElement{FunctionTypeID: 50}

		// Place the offfset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)

		// Now emit the code.
		require.NoError(t, compiler.compileCallIndirect(targetOperation))

		// We expect to exit from the code in callIndirect so the subsequet code must be unreachable.
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeTypeMismatchOnIndirectCall, env.jitStatus())
	})

	t.Run("ok", func(t *testing.T) {
		for _, growCallFrameStack := range []bool{false, true} {
			growCallFrameStack := growCallFrameStack
			t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
				targetType := &wasm.FunctionType{
					Params:  []wasm.ValueType{},
					Results: []wasm.ValueType{wasm.ValueTypeI32}}
				targetTypeID := wasm.FunctionTypeID(10) // Arbitrary number is fine for testing.
				operation := &wazeroir.OperationCallIndirect{TypeIndex: 0}

				// Ensure that the module instance has the type information for targetOperation.TypeIndex,
				// and the typeID  matches the table[targetOffset]'s type ID.
				moduleInstance := &wasm.ModuleInstance{Types: make([]*wasm.TypeInstance, 100)}
				moduleInstance.Types[operation.TableIndex] = &wasm.TypeInstance{Type: targetType, TypeID: targetTypeID}

				table := make([]wasm.TableElement, 10)
				for i := 0; i < len(table); i++ {
					table[i] = wasm.TableElement{FunctionAddress: wasm.FunctionAddress(i), FunctionTypeID: targetTypeID}
				}

				for i := 0; i < len(table); i++ {
					env := newJITEnvironment()
					env.setTable(table)
					engine := env.engine()

					// First we create the call target function with function address = i,
					// and it returns one value.
					expectedReturnValue := uint32(i * 1000)

					// We have to do compilation in a separate subtest since each compilation takes
					// the mutext lock and must release on the cleanup of each subtest.
					// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
					t.Run(fmt.Sprintf("compiling call target for %d", i), func(t *testing.T) {
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
						require.NoError(t, err)
						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						cf := &compiledFunction{
							codeSegment:        code,
							codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
						}
						engine.addCompiledFunction(table[i].FunctionAddress, cf)
					})

					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						if growCallFrameStack {
							env.setCallFrameStackPointer(engine.globalContext.callFrameStackLen - 1)
							env.setPreviousCallFrameStackPointer(engine.globalContext.callFrameStackLen - 1)
						}

						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						compiler.f = &wasm.FunctionInstance{ModuleInstance: moduleInstance, FunctionKind: wasm.FunctionKindWasm}

						// Place the offfset value. Here we try calling a function of functionaddr == table[i].FunctionAddress.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(i)})
						require.NoError(t, err)

						// At this point, we should have one item (offset value) on the stack.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						require.NoError(t, compiler.compileCallIndirect(operation))

						// At this point, we consumed the offset value, but the function returns one value,
						// so the stack pointer results in the same.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						// Generate the code under test and run.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)
						env.exec(code)

						if growCallFrameStack {
							// If the call frame stack pointer equals the length of call frame stack length,
							// we have to call the builtin function to grow the slice.
							require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
							require.Equal(t, builtinFunctionAddressGrowCallFrameStack, env.functionCallAddress(), env.functionCallAddress())

							// Grow the callFrame stack, and exec again from the return address.
							env.engine().builtinFunctionGrowCallFrameStack()
							jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.engine())))
						}

						require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
						require.Equal(t, uint64(1), env.stackPointer())
						require.Equal(t, expectedReturnValue, env.stackTopAsUint32())
					})
				}
			})
		}
	})
}

func TestArm64Compiler_compileSelect(t *testing.T) {
	for _, isFloat := range []bool{false, true} {
		isFloat := isFloat
		t.Run(fmt.Sprintf("float=%v", isFloat), func(t *testing.T) {
			for _, vals := range [][2]uint64{
				{1, 2}, {0, 1}, {1, 0},
				{math.Float64bits(-1), math.Float64bits(-1)},
				{math.Float64bits(-1), math.Float64bits(1)},
				{math.Float64bits(1), math.Float64bits(-1)},
			} {
				vals := vals
				t.Run(fmt.Sprintf("x1=%x,x2=%x", vals[0], vals[1]), func(t *testing.T) {
					for _, selectX1 := range []bool{false, true} {
						selectX1 := selectX1
						t.Run(fmt.Sprintf("select x1=%v", selectX1), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Push the select targets.
							for _, val := range vals {
								if isFloat {
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
								} else {
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
								}
								require.NoError(t, err)
							}

							// Push the selection seed.
							if selectX1 {
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 1})
							} else {
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
							}
							require.NoError(t, err)

							err = compiler.compileSelect()
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							code, _, _, err := compiler.compile()
							require.NoError(t, err)

							env.exec(code)
							require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())

							// Check if the correct value is chosen.
							if selectX1 {
								require.Equal(t, vals[0], env.stackTopAsUint64())
							} else {
								require.Equal(t, vals[1], env.stackTopAsUint64())
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compileSwap(t *testing.T) {
	const x, y uint64 = 100, 200
	op := &wazeroir.OperationSwap{Depth: 10}

	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Setup the initial values on the stack would look like: [y, ...., x]
	err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: y})
	require.NoError(t, err)
	// Push the middle dummy values.
	for i := 0; i < op.Depth-1; i++ {
		compiler.locationStack.pushValueLocationOnStack()
	}
	err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x})
	require.NoError(t, err)

	err = compiler.compileSwap(op)
	require.NoError(t, err)

	// After the swap, both values must be on registers.
	require.True(t, compiler.locationStack.peek().onRegister())
	require.True(t, compiler.locationStack.stack[0].onRegister())

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test and run.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, uint64(op.Depth+1), env.stackPointer())
	// y must be on the top due to Swap.
	require.Equal(t, y, env.stackTopAsUint64())
	// x must be on the bottom.
	require.Equal(t, x, env.stack()[0])
}

func TestArm64Compiler_compileModuleContextInitialization(t *testing.T) {
	for _, tc := range []struct {
		name           string
		moduleInstance *wasm.ModuleInstance
	}{
		{
			name: "no nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
			},
		},
		{
			name: "globals nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
			},
		},
		{
			name: "memory nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{Val: 100}},
				Tables:  []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
			},
		},
		{
			name: "table nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: nil}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "table empty",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 0)}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "memory zero length",
			moduleInstance: &wasm.ModuleInstance{
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 0)}},
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 0)},
			},
		},
		{
			name:           "nil",
			moduleInstance: &wasm.ModuleInstance{},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = tc.moduleInstance

			// The assembler skips the first instruction so we intentionally add NOP here.
			// TODO: delete after #233
			compiler.compileNOP()

			err := compiler.compileModuleContextInitialization()
			require.NoError(t, err)
			require.Empty(t, compiler.locationStack.usedRegisters)

			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			env.exec(code)

			// Check the exit status.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())

			// Check if the fields of engine.moduleContext are updated.
			engine := env.engine()

			bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Globals))
			require.Equal(t, bufSliceHeader.Data, engine.moduleContext.globalElement0Address)

			if tc.moduleInstance.MemoryInstance != nil {
				bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.MemoryInstance.Buffer))
				require.Equal(t, uint64(bufSliceHeader.Len), engine.moduleContext.memorySliceLen)
				require.Equal(t, bufSliceHeader.Data, engine.moduleContext.memoryElement0Address)
			}

			if len(tc.moduleInstance.Tables) > 0 {
				tableHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Tables[0].Table))
				require.Equal(t, uint64(tableHeader.Len), engine.moduleContext.tableSliceLen)
				require.Equal(t, tableHeader.Data, engine.moduleContext.tableElement0Address)
			}
		})
	}
}

func TestArm64Compiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			// Compiler needs global type information at compilation time.
			compiler.f.ModuleInstance = env.moduleInstance

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: &wasm.GlobalType{ValType: tp}}}
			env.addGlobals(globals...)

			// Emit the code.
			err := compiler.compilePreamble()
			require.NoError(t, err)
			op := &wazeroir.OperationGlobalGet{Index: 1}
			err = compiler.compileGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := compiler.locationStack.peek()
			require.True(t, global.onRegister())
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isFloatRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isIntRegister(global.register))
			}
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run the code assembled above.
			env.exec(code)

			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, env.stack()[0])
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
		})
	}
}

func TestArm64Compiler_compileGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64,
		wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			// Compiler needs global type information at compilation time.
			compiler.f.ModuleInstance = env.moduleInstance

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			env.addGlobals(nil, &wasm.GlobalInstance{Val: 40, Type: &wasm.GlobalType{ValType: tp}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Place the set target value.
			loc := compiler.locationStack.pushValueLocationOnStack()
			switch tp {
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			}
			env.stack()[loc.stackPointer] = valueToSet

			op := &wazeroir.OperationGlobalSet{Index: 1}
			err = compiler.compileGlobalSet(op)
			require.Equal(t, uint64(0), compiler.locationStack.sp)
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// The global value should be set to valueToSet.
			require.Equal(t, valueToSet, env.getGlobal(op.Index))
			// Plus we consumed the top of the stack, the stack pointer must be decremented.
			require.Equal(t, uint64(0), env.stackPointer())
		})
	}
}

func TestArm64Compiler_compileMemoryAccessOffsetSetup(t *testing.T) {
	bases := []uint32{0, 1 << 5, 1 << 9, 1 << 10, 1 << 15, math.MaxUint32 - 1, math.MaxUint32}
	offsets := []uint32{
		0, 1 << 10, 1 << 31,
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
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					reg, err := compiler.compileMemoryAccessOffsetSetup(offset, targetSizeInByte)
					require.NoError(t, err)

					compiler.locationStack.pushValueLocationOnRegister(reg)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate the code under test and run.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					mem := env.memory()
					if ceil := int64(base) + int64(offset) + int64(targetSizeInByte); int64(len(mem)) < ceil {
						// If the targe memory region's ceil exceeds the length of memory, we must exit the function
						// with jitCallStatusCodeMemoryOutOfBounds status code.
						require.Equal(t, jitCallStatusCodeMemoryOutOfBounds, env.jitStatus())
					} else {
						require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
						require.Equal(t, uint64(1), env.stackPointer())
						require.Equal(t, uint64(ceil-targetSizeInByte), env.stackTopAsUint64())
					}
				})
			}
		}
	}
}

func TestArm64Compiler_compileStore(t *testing.T) {
	// For testing. Arbitrary number is fine.
	storeTargetValue := uint64(math.MaxUint64)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryImmediate{Offset: 361}
	offset := arg.Offset + baseOffset

	for _, tc := range []struct {
		name                string
		isFloatTarget       bool
		targetSizeInBytes   uint32
		operationSetupFn    func(t *testing.T, compiler *arm64Compiler)
		storedValueVerifyFn func(t *testing.T, mem []byte)
	}{
		{
			name:              "i32.store",
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileStore32(&wazeroir.OperationStore32{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)

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
			require.Len(t, compiler.locationStack.usedRegisters, 0)
			require.Equal(t, uint64(0), compiler.locationStack.sp)

			// Generate the code under test.
			compiler.compileReturnFunction()
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Set the value on the left and right neighboring memoryregion,
			// so that we can veirfy the operation doesn't affect there.
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

func TestArm64Compiler_compileLoad(t *testing.T) {
	// For testing. Arbitrary number is fine.
	loadTargetValue := uint64(0x12_34_56_78_9a_bc_ef_fe)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryImmediate{Offset: 361}
	offset := baseOffset + arg.Offset

	for _, tc := range []struct {
		name                string
		isFloatTarget       bool
		operationSetupFn    func(t *testing.T, compiler *arm64Compiler)
		loadedValueVerifyFn func(t *testing.T, loadedValueAsUint64 uint64)
	}{
		{
			name: "i32.load",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadTargetValue), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, loadTargetValue, loadedValueAsUint64)
			},
		},
		{
			name: "f32.load",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
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
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int8(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load8u",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(byte(loadedValueAsUint64)), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8s",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int8(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8u",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(byte(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i32.load16s",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int16(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load16u",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadedValueAsUint64), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16s",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int16(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16u",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint16(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i64.load32s",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: true})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int32(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load32u",
			operationSetupFn: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: false})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint32(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
	} {

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			err := compiler.compilePreamble()
			require.NoError(t, err)

			binary.LittleEndian.PutUint64(env.memory()[offset:], loadTargetValue)

			// Before load operation, we must push the base offset value.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: baseOffset})
			require.NoError(t, err)

			tc.operationSetupFn(t, compiler)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			require.Equal(t, uint64(1), compiler.locationStack.sp)
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			loadedLocation := compiler.locationStack.peek()
			require.True(t, loadedLocation.onRegister())
			if tc.isFloatTarget {
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedLocation.registerType())
			} else {
				require.Equal(t, generalPurposeRegisterTypeInt, loadedLocation.registerType())
			}
			compiler.compileReturnFunction()

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Verify the loaded value.
			require.Equal(t, uint64(1), env.stackPointer())
			tc.loadedValueVerifyFn(t, env.stackTopAsUint64())
		})
	}
}

func TestArm64Compiler_compileMemoryGrow(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	err = compiler.compileMemoryGrow()
	require.NoError(t, err)

	// Emit arbitrary code after MemoryGrow returned so that we can verify
	// that the code can set the return address properly.
	const expValue uint32 = 100
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
	require.NoError(t, err)
	compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// After the initial exec, the code must exit with builtin function call status and funcaddress for memory grow.
	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
	require.Equal(t, builtinFunctionAddressMemoryGrow, env.functionCallAddress())

	// Reenter from the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.engine())))

	// Check if the code successfully executed the code after builtin function call.
	require.Equal(t, expValue, env.stackTopAsUint32())
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestArm64Compiler_compileMemorySize(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Emit memory.size instructions.
	err = compiler.compileMemorySize()
	require.NoError(t, err)
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.locationStack.sp)
	require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().registerType())

	compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	require.Equal(t, uint32(defaultMemoryPageNumInTest), env.stackTopAsUint32())
}

func TestArm64Compiler_compileMaybeGrowValueStack(t *testing.T) {
	t.Run("not grow", func(t *testing.T) {
		const stackPointerCeil = 5
		for _, baseOffset := range []uint64{5, 10, 20} {
			t.Run(fmt.Sprintf("%d", baseOffset), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)

				// The assembler skips the first instruction so we intentionally add NOP here.
				// TODO: delete after #233
				compiler.compileNOP()

				err := compiler.compileMaybeGrowValueStack()
				require.NoError(t, err)
				require.NotNil(t, compiler.onStackPointerCeilDeterminedCallBack)

				valueStackLen := uint64(len(env.stack()))
				stackBasePointer := valueStackLen - baseOffset // Ceil <= valueStackLen - stackBasePointer = no need to grow!
				compiler.onStackPointerCeilDeterminedCallBack(stackPointerCeil)
				compiler.onStackPointerCeilDeterminedCallBack = nil
				env.setValueStackBasePointer(stackBasePointer)

				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// The status code must be "Returned", not "BuiltinFunctionCall".
				require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			})
		}
	})
	t.Run("grow", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		// The assembler skips the first instruction so we intentionally add NOP here.
		// TODO: delete after #233
		compiler.compileNOP()

		err := compiler.compileMaybeGrowValueStack()
		require.NoError(t, err)

		// On the return from grow value stack, we simply return.
		compiler.compileReturnFunction()

		stackPointerCeil := uint64(6)
		compiler.stackPointerCeil = stackPointerCeil
		valueStackLen := uint64(len(env.stack()))
		stackBasePointer := valueStackLen - 5 // Ceil > valueStackLen - stackBasePointer = need to grow!
		env.setValueStackBasePointer(stackBasePointer)

		// Generate and run the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		// Check if the call exits with builtin function call status.
		require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())

		// Reenter from the return address.
		returnAddress := env.callFrameStackPeek().returnAddress
		require.NotZero(t, returnAddress)
		jitcall(returnAddress, uintptr(unsafe.Pointer(env.engine())))

		// Check the result. This should be "Returned".
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestArm64Compiler_compileHostFunction(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)

	// The assembler skips the first instruction so we intentionally add NOP here.
	// TODO: delete after #233
	compiler.compileNOP()

	addr := wasm.FunctionAddress(100)
	err := compiler.compileHostFunction(addr)
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// On the return, the code must exit with the host call status and the specified call address.
	require.Equal(t, jitCallStatusCodeCallHostFunction, env.jitStatus())
	require.Equal(t, addr, env.functionCallAddress())

	// Re-enter the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.engine())))

	// After that, the code must exit with returned status.
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestArm64Compiler_compile_Clz_Ctz_Popcnt(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindClz,
		wazeroir.OperationKindCtz,
		wazeroir.OperationKindPopcnt,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, tp := range []wazeroir.UnsignedInt{wazeroir.UnsignedInt32, wazeroir.UnsignedInt64} {
				tp := tp
				is32bit := tp == wazeroir.UnsignedInt32
				t.Run(tp.String(), func(t *testing.T) {
					for _, v := range []uint64{
						0, 1, 1 << 4, 1 << 6, 1 << 31,
						0b11111111110000, 0b010101010, 0b1111111111111, math.MaxUint64,
					} {
						name := fmt.Sprintf("%064b", v)
						if is32bit {
							name = fmt.Sprintf("%032b", v)
						}
						t.Run(name, func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							if is32bit {
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
							} else {
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)

							switch kind {
							case wazeroir.OperationKindClz:
								err = compiler.compileClz(&wazeroir.OperationClz{Type: tp})
							case wazeroir.OperationKindCtz:
								err = compiler.compileCtz(&wazeroir.OperationCtz{Type: tp})
							case wazeroir.OperationKindPopcnt:
								err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: tp})
							}
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Generate and run the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// One value must be pushed as a result.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindClz:
								if is32bit {
									require.Equal(t, bits.LeadingZeros32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.LeadingZeros64(v), int(env.stackTopAsUint32()))
								}
							case wazeroir.OperationKindCtz:
								if is32bit {
									require.Equal(t, bits.TrailingZeros32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.TrailingZeros64(v), int(env.stackTopAsUint32()))
								}
							case wazeroir.OperationKindPopcnt:
								if is32bit {
									require.Equal(t, bits.OnesCount32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.OnesCount64(v), int(env.stackTopAsUint32()))
								}
							}
						})
					}
				})
			}
		})
	}
}
func TestArm64Compiler_compile_Div_Rem(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindDiv,
		wazeroir.OperationKindRem,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, signedType := range []wazeroir.SignedType{
				wazeroir.SignedTypeUint32,
				wazeroir.SignedTypeUint64,
				wazeroir.SignedTypeInt32,
				wazeroir.SignedTypeInt64,
				wazeroir.SignedTypeFloat32,
				wazeroir.SignedTypeFloat64,
			} {
				signedType := signedType
				t.Run(signedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1234, 5}, {5, 1234}, {4, 2}, {40, 4}, {123456, 4},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{0x80000000, 0xffffffff},                 // This is equivalent to (-2^31 / -1) and results in overflow for 32-bit signed div.
						{0x8000000000000000, 0xffffffffffffffff}, // This is equivalent to (-2^63 / -1) and results in overflow for 64-bit signed div.
						{0xffffffff /* -1 in signed 32bit */, 0xfffffffe /* -2 in signed 32bit */},
						{0xffffffffffffffff /* -1 in signed 64bit */, 0xfffffffffffffffe /* -2 in signed 64bit */},
						{1, 0xffff_ffff_ffff_ffff},
						{math.Float64bits(1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(1.11231), math.Float64bits(-12312312.12312)},
						{math.Float64bits(-1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(-1.11231), math.Float64bits(-12312312.12312)},
						{math.Float64bits(1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(-12312312.12312), math.Float64bits(1.11231)},
						{math.Float64bits(12312312.12312), math.Float64bits(-1.11231)},
						{math.Float64bits(-12312312.12312), math.Float64bits(-1.11231)},
						{1, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
						{math.Float64bits(math.MaxFloat32), 1},
						{math.Float64bits(math.SmallestNonzeroFloat32), 1},
						{math.Float64bits(math.MaxFloat64), 1},
						{math.Float64bits(math.SmallestNonzeroFloat64), 1},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 0},
						{math.Float64bits(math.Inf(-1)), 0},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									// In order to test zero value on non-zero register, we directly assign an register.
									reg, err := compiler.allocateRegister(generalPurposeRegisterTypeInt)
									require.NoError(t, err)
									compiler.locationStack.pushValueLocationOnRegister(reg)
									compiler.compileConstToRegisterInstruction(arm64.AMOVD, int64(v), reg)
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist for comparison.
							require.Equal(t, uint64(2), compiler.locationStack.sp)

							switch kind {
							case wazeroir.OperationKindDiv:
								err = compiler.compileDiv(&wazeroir.OperationDiv{Type: signedType})
							case wazeroir.OperationKindRem:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt32})
								case wazeroir.SignedTypeInt64:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt64})
								case wazeroir.SignedTypeUint32:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint32})
								case wazeroir.SignedTypeUint64:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint64})
								case wazeroir.SignedTypeFloat32:
									// Rem undefined for float32.
									t.Skip()
								case wazeroir.SignedTypeFloat64:
									// Rem undefined for float64.
									t.Skip()
								}
							}
							require.NoError(t, err)

							compiler.compileReturnFunction()

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							switch kind {
							case wazeroir.OperationKindDiv:
								switch signedType {
								case wazeroir.SignedTypeUint32:
									if uint32(x2) == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, uint32(x1)/uint32(x2), env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeInt32:
									v1, v2 := int32(x1), int32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else if v1 == math.MinInt32 && v2 == -1 {
										require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									} else {
										require.Equal(t, v1/v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, x1/x2, env.stackTopAsUint64())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else if v1 == math.MinInt64 && v2 == -1 {
										require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									} else {
										require.Equal(t, v1/v2, env.stackTopAsInt64())
									}
								case wazeroir.SignedTypeFloat32:
									exp := math.Float32frombits(uint32(x1)) / math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.SignedTypeFloat64:
									exp := math.Float64frombits(x1) / math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindRem:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									v1, v2 := int32(x1), int32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt64())
									}
								case wazeroir.SignedTypeUint32:
									v1, v2 := uint32(x1), uint32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, x1%x2, env.stackTopAsUint64())
									}

								}
							}
						})
					}
				})
			}
		})
	}
}
func TestArm64Compiler_compile_Abs_Neg_Ceil_Floor_Trunc_Nearest_Sqrt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler *arm64Compiler)
		verifyFunc func(t *testing.T, v float64, raw uint64)
	}{
		{
			name:    "abs-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileAbs(&wazeroir.OperationAbs{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Abs(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "abs-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileAbs(&wazeroir.OperationAbs{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Abs(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "neg-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileNeg(&wazeroir.OperationNeg{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := -float32(v)
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "neg-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileNeg(&wazeroir.OperationNeg{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := -v
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "ceil-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileCeil(&wazeroir.OperationCeil{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Ceil(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "ceil-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileCeil(&wazeroir.OperationCeil{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Ceil(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "floor-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileFloor(&wazeroir.OperationFloor{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Floor(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "floor-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileFloor(&wazeroir.OperationFloor{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Floor(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "trunc-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileTrunc(&wazeroir.OperationTrunc{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Trunc(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "trunc-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileTrunc(&wazeroir.OperationTrunc{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Trunc(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "nearest-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileNearest(&wazeroir.OperationNearest{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := moremath.WasmCompatNearestF32(float32(v))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "nearest-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileNearest(&wazeroir.OperationNearest{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := moremath.WasmCompatNearestF64(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "sqrt-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileSqrt(&wazeroir.OperationSqrt{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Sqrt(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "sqrt-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileSqrt(&wazeroir.OperationSqrt{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Sqrt(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range []float64{
				0, 1 << 63, 1<<63 | 12345, 1 << 31,
				1<<31 | 123455, 6.8719476736e+10,
				// This verifies that the impl is Wasm compatible in nearest, rather than being equivalent of math.Round.
				// See moremath.WasmCompatNearestF32 and moremath.WasmCompatNearestF64
				-4.5,
				1.37438953472e+11, -1.3,
				-1231.123, 1.3, 100.3, -100.3, 1231.123,
				math.Inf(1), math.Inf(-1), math.NaN(),
			} {
				v := v
				t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					if tc.is32bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(v)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					require.Equal(t, uint64(1), compiler.locationStack.sp)
					require.Len(t, compiler.locationStack.usedRegisters, 1)

					tc.setupFunc(t, compiler)

					// We consumed one value, but push the result after operation.
					require.Equal(t, uint64(1), compiler.locationStack.sp)
					require.Len(t, compiler.locationStack.usedRegisters, 1)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer()) // Result must be pushed!

					tc.verifyFunc(t, v, env.stackTopAsUint64())
				})
			}
		})
	}
}

func TestArm64Compiler_compile_Min_Max_Copysign(t *testing.T) {
	for _, tc := range []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler *arm64Compiler)
		verifyFunc func(t *testing.T, x1, x2 float64, raw uint64)
	}{
		{
			name:    "min-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileMin(&wazeroir.OperationMin{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(moremath.WasmCompatMin(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "min-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileMin(&wazeroir.OperationMin{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMin(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileMax(&wazeroir.OperationMax{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(moremath.WasmCompatMax(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileMax(&wazeroir.OperationMax{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMax(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileCopysign(&wazeroir.OperationCopysign{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(math.Copysign(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "copysign-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler *arm64Compiler) {
				err := compiler.compileCopysign(&wazeroir.OperationCopysign{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := math.Copysign(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, vs := range [][2]float64{
				{100, -1.1}, {100, 0}, {0, 0}, {1, 1},
				{-1, 100}, {100, 200}, {100.01234124, 100.01234124},
				{100.01234124, -100.01234124}, {200.12315, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 1.37438953472e+11 /* = 1 << 37*/},
				{math.Inf(1), 100}, {math.Inf(1), -100},
				{100, math.Inf(1)}, {-100, math.Inf(1)},
				{math.Inf(-1), 100}, {math.Inf(-1), -100},
				{100, math.Inf(-1)}, {-100, math.Inf(-1)},
				{math.Inf(1), 0}, {math.Inf(-1), 0},
				{0, math.Inf(1)}, {0, math.Inf(-1)},
				{math.NaN(), 0}, {0, math.NaN()},
				{math.NaN(), 12321}, {12313, math.NaN()},
				{math.NaN(), math.NaN()},
			} {
				x1, x2 := vs[0], vs[1]
				t.Run(fmt.Sprintf("x1=%f_x2=%f", x1, x2), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the target values.
					if tc.is32bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(x1)})
						require.NoError(t, err)
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: x1})
						require.NoError(t, err)
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: x2})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					require.Equal(t, uint64(2), compiler.locationStack.sp)
					require.Len(t, compiler.locationStack.usedRegisters, 2)

					tc.setupFunc(t, compiler)

					// We consumed two values, but push one value after operation.
					require.Equal(t, uint64(1), compiler.locationStack.sp)
					require.Len(t, compiler.locationStack.usedRegisters, 1)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer()) // Result must be pushed!

					tc.verifyFunc(t, x1, x2, env.stackTopAsUint64())
				})
			}
		})
	}
}

func TestArm64Compiler_compileF32DemoteFromF64(t *testing.T) {
	for _, v := range []float64{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		6.8719476736e+10,  /* = 1 << 36 */
		1.37438953472e+11, /* = 1 << 37 */
		math.Inf(1), math.Inf(-1), math.NaN(),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the demote target.
			err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
			require.NoError(t, err)

			err = compiler.compileF32DemoteFromF64()
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(v) {
				require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
			} else {
				exp := float32(v)
				actual := env.stackTopAsFloat32()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestArm64Compiler_compileF64PromoteFromF32(t *testing.T) {
	for _, v := range []float32{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the promote target.
			err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: v})
			require.NoError(t, err)

			err = compiler.compileF64PromoteFromF32()
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(float64(v)) {
				require.True(t, math.IsNaN(env.stackTopAsFloat64()))
			} else {
				exp := float64(v)
				actual := env.stackTopAsFloat64()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestArm64Compiler_compileReinterpret(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindF32ReinterpretFromI32,
		wazeroir.OperationKindF64ReinterpretFromI64,
		wazeroir.OperationKindI32ReinterpretFromF32,
		wazeroir.OperationKindI64ReinterpretFromF64,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, originOnStack := range []bool{false, true} {
				originOnStack := originOnStack
				t.Run(fmt.Sprintf("%v", originOnStack), func(t *testing.T) {
					for _, v := range []uint64{
						0, 1, 1 << 16, 1 << 31, 1 << 32, 1 << 63,
						math.MaxInt32, math.MaxUint32, math.MaxUint64,
					} {
						v := v
						t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							if originOnStack {
								loc := compiler.locationStack.pushValueLocationOnStack()
								env.stack()[loc.stackPointer] = v
								env.setStackPointer(1)
							}

							var is32Bit bool
							switch kind {
							case wazeroir.OperationKindF32ReinterpretFromI32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
									require.NoError(t, err)
								}
								err = compiler.compileF32ReinterpretFromI32()
								require.NoError(t, err)
							case wazeroir.OperationKindF64ReinterpretFromI64:
								if !originOnStack {
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
									require.NoError(t, err)
								}
								err = compiler.compileF64ReinterpretFromI64()
								require.NoError(t, err)
							case wazeroir.OperationKindI32ReinterpretFromF32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
									require.NoError(t, err)
								}
								err = compiler.compileI32ReinterpretFromF32()
								require.NoError(t, err)
							case wazeroir.OperationKindI64ReinterpretFromF64:
								if !originOnStack {
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
									require.NoError(t, err)
								}
								err = compiler.compileI64ReinterpretFromF64()
								require.NoError(t, err)
							default:
								t.Fail()
							}

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Generate and run the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Reinterpret must preserve the bit-pattern.
							if is32Bit {
								require.Equal(t, uint32(v), env.stackTopAsUint32())
							} else {
								require.Equal(t, v, env.stackTopAsUint64())
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compileExtend(t *testing.T) {
	for _, signed := range []bool{false, true} {
		signed := signed
		t.Run(fmt.Sprintf("signed=%v", signed), func(t *testing.T) {
			for _, v := range []uint32{
				0, 1, 1 << 14, 1 << 31, math.MaxUint32, 0xFFFFFFFF, math.MaxInt32,
			} {
				v := v
				t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the promote target.
					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: v})
					require.NoError(t, err)

					err = compiler.compileExtend(&wazeroir.OperationExtend{Signed: signed})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, uint64(1), env.stackPointer())
					if signed {
						expected := int64(int32(v))
						require.Equal(t, expected, env.stackTopAsInt64())
					} else {
						expected := uint64(uint32(v))
						require.Equal(t, expected, env.stackTopAsUint64())
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compileITruncFromF(t *testing.T) {
	for _, tc := range []struct {
		outputType wazeroir.SignedInt
		inputType  wazeroir.Float
	}{
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []float64{
				1.0, 100, -100, 1, -1, 100.01234124, -100.01234124, 200.12315,
				6.8719476736e+10 /* = 1 << 36 */, -6.8719476736e+10, 1.37438953472e+11, /* = 1 << 37 */
				-1.37438953472e+11, -2147483649.0, 2147483648.0, math.MinInt32,
				math.MaxInt32, math.MaxUint32, math.MinInt64, math.MaxInt64,
				math.MaxUint64, math.MaxFloat32, math.SmallestNonzeroFloat32, math.MaxFloat64,
				math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.NaN(),
			} {
				v := v
				if v == math.MaxInt32 {
					// Note that math.MaxInt32 is rounded up to math.MaxInt32+1 in 32-bit float representation.
					require.Equal(t, float32(2147483648.0) /* = math.MaxInt32+1 */, float32(v))
				} else if v == math.MaxUint32 {
					// Note that math.MaxUint32 is rounded up to math.MaxUint32+1 in 32-bit float representation.
					require.Equal(t, float32(4294967296 /* = math.MaxUint32+1 */), float32(v))
				} else if v == math.MaxInt64 {
					// Note that math.MaxInt64 is rounded up to math.MaxInt64+1 in 32/64-bit float representation.
					require.Equal(t, float32(9223372036854775808.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(9223372036854775808.0) /* = math.MaxInt64+1 */, float64(v))
				} else if v == math.MaxUint64 {
					// Note that math.MaxUint64 is rounded up to math.MaxUint64+1 in 32/64-bit float representation.
					require.Equal(t, float32(18446744073709551616.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(18446744073709551616.0) /* = math.MaxInt64+1 */, float64(v))
				}

				t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the conversion target.
					if tc.inputType == wazeroir.Float32 {
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(v)})
					} else {
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
					}
					require.NoError(t, err)

					err = compiler.compileITruncFromF(&wazeroir.OperationITruncFromF{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					expStatus := jitCallStatusCodeReturned
					if math.IsNaN(v) {
						expStatus = jitCallStatusCodeInvalidFloatToIntConversion
					}
					if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt32 {
						f32 := float32(v)
						if f32 < math.MinInt32 || f32 >= math.MaxInt32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int32(math.Trunc(float64(f32))), env.stackTopAsInt32())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt64 {
						f32 := float32(v)
						if f32 < math.MinInt64 || f32 >= math.MaxInt64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int64(math.Trunc(float64(f32))), env.stackTopAsInt64())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt32 {
						if v < math.MinInt32 || v > math.MaxInt32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int32(math.Trunc(v)), env.stackTopAsInt32())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt64 {
						if v < math.MinInt64 || v >= math.MaxInt64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int64(math.Trunc(v)), env.stackTopAsInt64())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint32 {
						f32 := float32(v)
						if f32 < 0 || f32 >= math.MaxUint32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint32(math.Trunc(float64(f32))), env.stackTopAsUint32())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint32 {
						if v < 0 || v > math.MaxUint32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint32(math.Trunc(v)), env.stackTopAsUint32())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint64 {
						f32 := float32(v)
						if f32 < 0 || f32 >= math.MaxUint64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint64(math.Trunc(float64(f32))), env.stackTopAsUint64())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint64 {
						if v < 0 || v >= math.MaxUint64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint64(math.Trunc(v)), env.stackTopAsUint64())
						}
					}
					require.Equal(t, expStatus, env.jitStatus())
				})
			}
		})
	}
}

func TestArm64Compiler_compileFConvertFromI(t *testing.T) {
	for _, tc := range []struct {
		inputType  wazeroir.SignedInt
		outputType wazeroir.Float
	}{
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []uint64{
				0, 1, 12345, 1 << 31, 1 << 32, 1 << 54, 1 << 63,
				0xffff_ffff_ffff_ffff, 0xffff_ffff,
				0xffff_ffff_ffff_fffe, 0xffff_fffe,
				math.MaxUint32, math.MaxUint64, math.MaxInt32, math.MaxInt64,
			} {
				t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the conversion target.
					if tc.inputType == wazeroir.SignedInt32 || tc.inputType == wazeroir.SignedUint32 {
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
					} else {
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(v)})
					}
					require.NoError(t, err)

					err = compiler.compileFConvertFromI(&wazeroir.OperationFConvertFromI{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					require.Equal(t, uint64(1), env.stackPointer())
					actualBits := env.stackTopAsUint64()
					if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt32 {
						exp := float32(int32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt64 {
						exp := float32(int64(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt32 {
						exp := float64(int32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt64 {
						exp := float64(int64(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint32 {
						exp := float32(uint32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint32 {
						exp := float64(uint32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint64 {
						exp := float32(v)
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint64 {
						exp := float64(v)
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileBrTable(t *testing.T) {
	requireRunAndExpectedValueReturned := func(t *testing.T, c *arm64Compiler, expValue uint32) {
		// Emit code for each label which returns the frame ID.
		for returnValue := uint32(0); returnValue < 7; returnValue++ {
			label := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			c.ir.LabelCallers[label.String()] = 1
			_ = c.compileLabel(&wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(&wazeroir.OperationConstI32{Value: label.FrameID})
			err := c.compileReturnFunction()
			require.NoError(t, err)
		}

		// Generate the code under test and run.
		env := newJITEnvironment()
		code, _, _, err := c.compile()
		require.NoError(t, err)
		env.exec(code)

		// Check the returned value.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expValue, env.stackTopAsUint32())
	}

	getBranchTargetDropFromFrameID := func(frameid uint32) *wazeroir.BranchTargetDrop {
		return &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{
			Label: &wazeroir.Label{FrameID: frameid, Kind: wazeroir.LabelKindHeader}},
		}
	}

	for _, tc := range []struct {
		name          string
		index         int64
		o             *wazeroir.OperationBrTable
		expectedValue uint32
	}{
		{
			name:          "only default with index 0",
			o:             &wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         0,
			expectedValue: 6,
		},
		{
			name:          "only default with index 100",
			o:             &wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         100,
			expectedValue: 6,
		},
		{
			name: "select default with targets and good index",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         3,
			expectedValue: 6,
		},
		{
			name: "select default with targets and huge index",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         100000,
			expectedValue: 6,
		},
		{
			name: "select first with two targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         0,
			expectedValue: 1,
		},
		{
			name: "select last with two targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         1,
			expectedValue: 2,
		},
		{
			name: "select first with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         0,
			expectedValue: 1,
		},
		{
			name: "select middle with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         2,
			expectedValue: 3,
		},
		{
			name: "select last with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         4,
			expectedValue: 5,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.ir = &wazeroir.CompilationResult{LabelCallers: map[string]uint32{}}

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.index)})
			require.NoError(t, err)

			err = compiler.compileBrTable(tc.o)
			require.NoError(t, err)

			require.Len(t, compiler.locationStack.usedRegisters, 0)

			requireRunAndExpectedValueReturned(t, compiler, tc.expectedValue)
		})
	}
}
