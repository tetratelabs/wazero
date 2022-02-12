//go:build arm64
// +build arm64

package jit

import (
	"context"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
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
	cmp, err := newCompiler(&wasm.FunctionInstance{ModuleInstance: j.moduleInstance}, nil)
	require.NoError(t, err)
	ret, ok := cmp.(*arm64Compiler)
	require.True(t, ok)
	ret.labels = make(map[string]*labelInfo)
	ret.ir = &wazeroir.CompilationResult{}
	return ret
}

// TODO: delete this as this could be a duplication from other tests especially spectests.
// Use this until we could run spectests on arm64.
func TestArm64CompilerEndToEnd(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		body []byte
		sig  *wasm.FunctionType
	}{
		{name: "empty", body: []byte{wasm.OpcodeEnd}, sig: &wasm.FunctionType{}},
		{name: "br .return", body: []byte{wasm.OpcodeBr, 0, wasm.OpcodeEnd}, sig: &wasm.FunctionType{}},
		{
			name: "consts",
			body: []byte{
				wasm.OpcodeI32Const, 1, wasm.OpcodeI64Const, 1,
				wasm.OpcodeF32Const, 1, 1, 1, 1, wasm.OpcodeF64Const, 1, 2, 3, 4, 5, 6, 7, 8,
				wasm.OpcodeEnd,
			},
			// We push four constants.
			sig: &wasm.FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeF32, wasm.ValueTypeF64}},
		},
		{
			name: "add",
			body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeI32Const, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd},
			sig:  &wasm.FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		{
			name: "sub",
			body: []byte{wasm.OpcodeI64Const, 1, wasm.OpcodeI64Const, 1, wasm.OpcodeI64Sub, wasm.OpcodeEnd},
			sig:  &wasm.FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI64}},
		},
		{
			name: "mul",
			body: []byte{wasm.OpcodeI64Const, 1, wasm.OpcodeI64Const, 1, wasm.OpcodeI64Mul, wasm.OpcodeEnd},
			sig:  &wasm.FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI64}},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			engine := newEngine()
			f := &wasm.FunctionInstance{
				FunctionType: &wasm.TypeInstance{Type: tc.sig},
				Body:         tc.body,
			}
			err := engine.Compile(f)
			require.NoError(t, err)
			_, err = engine.Call(ctx, f)
			require.NoError(t, err)
		})
	}
}

func TestArchContextOffsetInEngine(t *testing.T) {
	var eng engine
	// If this fails, we have to fix jit_arm64.s as well.
	require.Equal(t, int(unsafe.Offsetof(eng.jitCallReturnAddress)), engineArchContextJITCallReturnAddressOffset)
}

func TestArm64Compiler_returnFunction(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		env := newJITEnvironment()

		// Build code.
		compiler := env.requireNewCompiler(t)
		err := compiler.emitPreamble()
		require.NoError(t, err)
		compiler.returnFunction()

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run native code.
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
			//	Each function pushes its funcaddr and soon returns.
			compiler := env.requireNewCompiler(t)
			err := compiler.emitPreamble()
			require.NoError(t, err)

			// Push its funcaddr.
			expValue := uint32(funcaddr)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
			require.NoError(t, err)

			// And then return.
			err = compiler.returnFunction()
			require.NoError(t, err)

			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Compiles and adds to the engine.
			codeInitialAddress := uintptr(unsafe.Pointer(&code[0]))
			compiledFunction := &compiledFunction{codeSegment: code, codeInitialAddress: codeInitialAddress}
			engine.addCompiledFunction(funcaddr, compiledFunction)

			// Pushes the frame whose return address equals the beginning of the function just compiled ^.
			frame := callFrame{
				returnAddress: codeInitialAddress,
				// Note that return stack base pointer is set to funcaddr*10 and this is where the const should be pushed.
				returnStackBasePointer: uint64(funcaddr) * 10,
				compiledFunction:       compiledFunction,
			}
			engine.callFrameStack[engine.globalContext.callFrameStackPointer] = frame
			engine.globalContext.callFrameStackPointer++

			stackPointerToExpectedValue[frame.returnStackBasePointer] = expValue
		}

		require.Equal(t, uint64(callFrameNums), env.callFrameStackPointer())

		// Run codes.
		env.exec(engine.callFrameTop().compiledFunction.codeSegment)

		// Check the exit status.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())

		// Check the stack values.
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
			err := compiler.emitPreamble()

			expStackPointer := uint64(100)
			compiler.locationStack.sp = expStackPointer
			require.NoError(t, err)
			compiler.exit(s)

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
					err := compiler.emitPreamble()
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
					compiler.releaseRegisterToStack(loc)
					compiler.returnFunction()

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
			err := compiler.emitPreamble()
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
			compiler.releaseRegisterToStack(compiler.locationStack.peek())
			compiler.exit(jitCallStatusCodeReturned)

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

func TestArm64Compiler_loadValueOnStackToRegister(t *testing.T) {
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
			err := compiler.emitPreamble()
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
			compiler.loadValueOnStackToRegister(loc)
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			require.True(t, loc.onRegister())

			// To verify the behavior, increment the value on the register.
			if tc.isFloat {
				// For float, we cannot add consts, so load the constant first.
				err = compiler.emitFloatConstant(false, math.Float64bits(1))
				require.NoError(t, err)
				// Then, do the increment.
				compiler.applyRegisterToRegisterInstruction(arm64.AFADDD, compiler.locationStack.peek().register, loc.register)
				// Delete the loaded const.
				compiler.locationStack.pop()
			} else {
				compiler.applyConstToRegisterInstruction(arm64.AADD, 1, loc.register)
			}

			// Release the value to the memory stack so that we can see the value after exiting.
			compiler.releaseRegisterToStack(loc)
			compiler.exit(jitCallStatusCodeReturned)

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
							err := compiler.emitPreamble()
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
							compiler.loadConditionalRegisterToGeneralPurposeRegister(resultLocation)
							require.True(t, resultLocation.onRegister())

							// Release the value to the memory stack again to verify the operation.
							compiler.releaseRegisterToStack(resultLocation)
							compiler.returnFunction()

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
							err := compiler.emitPreamble()
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
							compiler.releaseRegisterToStack(resultLocation)
							compiler.returnFunction()

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
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) + math.Float64frombits(x2)
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
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) - math.Float64frombits(x2)
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
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) * math.Float64frombits(x2)
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

func TestArm64Compiler_compile_And_Or_Xor(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindAnd,
		wazeroir.OperationKindOr,
		wazeroir.OperationKindXor,
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
							err := compiler.emitPreamble()
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
							compiler.releaseRegisterToStack(resultLocation)
							compiler.returnFunction()

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
							}
						})
					}
				})
			}
		})
	}
}

func TestArm64Compiler_compielePick(t *testing.T) {
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
			err := compiler.emitPreamble()
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
			compiler.releaseRegisterToStack(pickedLocation)
			if tc.isPickTargetOnRegister {
				compiler.releaseRegisterToStack(pickTargetLocation)
			}
			compiler.returnFunction()

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

func TestArm64Compiler_compieleDrop(t *testing.T) {
	t.Run("range nil", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.emitPreamble()
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

		compiler.returnFunction()

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

		err := compiler.emitPreamble()
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
		compiler.releaseRegisterToStack(top)

		compiler.returnFunction()

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

		err := compiler.emitPreamble()
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
		err = compiler.releaseAllRegistersToStack()
		require.NoError(t, err)
		compiler.returnFunction()

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
		err := compiler.emitPreamble()
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
		compiler.exit(jitCallStatusCodeReturned)

		// Now emit the body. First we add NOP so that we can execute code after the target label.
		nop := compiler.newProg()
		nop.As = obj.ANOP
		compiler.addInstruction(nop)

		err := compiler.emitPreamble()
		require.NoError(t, err)

		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: backwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with unreachable status.
		compiler.exit(jitCallStatusCodeUnreachable)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		// .backwardLabel:
		//    exit jitCallStatusCodeReturned
		//    nop
		//    ... code from emitPreamble()
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
		err := compiler.emitPreamble()
		require.NoError(t, err)

		// Emit the forward br, meaning that handle Br instruction where the target label hasn't been compiled yet.
		forwardLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: forwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with Unreachable status.
		compiler.exit(jitCallStatusCodeUnreachable)

		// Emit code for the forward label where we emit the expectedValue and then exit.
		requireAddLabel(t, compiler, forwardLabel)
		compiler.exit(jitCallStatusCodeReturned)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		//    ... code from emitPreamble()
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
					err := compiler.emitPreamble()
					require.NoError(t, err)

					tc.setupFunc(t, compiler, shouldGoToElse)
					require.Equal(t, uint64(1), compiler.locationStack.sp)

					err = compiler.compileBrIf(&wazeroir.OperationBrIf{Then: thenBranchTarget, Else: elseBranchTarget})
					require.NoError(t, err)
					compiler.exit(unreachableStatus)

					// Emit code for .then label.
					requireAddLabel(t, compiler, thenBranchTarget.Target.Label)
					compiler.exit(thenLabelExitStatus)

					// Emit code for .else label.
					requireAddLabel(t, compiler, elseBranchTarget.Target.Label)
					compiler.exit(elseLabelExitStatus)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					// The generated code looks like this:
					//
					//    ... code from emitPreamble()
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
	t.Run("invalid", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.emitPreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.readInstructionAddress(obj.AJMP, reservedRegisterForTemporary)

		compiler.exit(jitCallStatusCodeReturned)

		// If generate the code without JMP after readInstructionAddress,
		// the call back added must return error.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.emitPreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		const addressReg = reservedRegisterForTemporary
		compiler.readInstructionAddress(obj.ARET, addressReg)

		// Jump to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		jmpToAfterRet := compiler.newProg()
		jmpToAfterRet.As = obj.AJMP
		jmpToAfterRet.To.Type = obj.TYPE_MEM
		jmpToAfterRet.To.Reg = addressReg
		compiler.addInstruction(jmpToAfterRet)

		compiler.exit(jitCallStatusCodeUnreachable)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the jmp instruction above
		// must target here.
		err = compiler.returnFunction()
		require.NoError(t, err)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}
