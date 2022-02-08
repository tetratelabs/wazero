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
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

func (j *jitEnv) requireNewCompiler(t *testing.T) *arm64Compiler {
	cmp, err := newCompiler(&wasm.FunctionInstance{ModuleInstance: j.moduleInstance}, nil)
	require.NoError(t, err)
	ret, ok := cmp.(*arm64Compiler)
	require.True(t, ok)
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

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code
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
			compiler.returnFunction()

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
			loc := compiler.locationStack.pushValueOnStack()
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
			compiler.returnFunction()

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

func TestArm64Compiler_compile_Le_Lt_Gt_Ge(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
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

							// At this point, two values exist.
							require.Equal(t, uint64(2), compiler.locationStack.sp)

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
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.locationStack.sp)
							resultLocation := compiler.locationStack.peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onConditionalRegister())

							compiler.loadConditionalFlagOnGeneralPurposeRegister(resultLocation)
							require.True(t, resultLocation.onRegister())

							// Release the value to the memory stack again to verify the operation, and then return.
							compiler.releaseRegisterToStack(resultLocation)
							compiler.returnFunction()

							// Generate the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)

							// Run code.
							env.exec(code)

							// Check the stack.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindLe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) <= int32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) <= uint32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) <= int64(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 <= x2, env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) <= math.Float32frombits(uint32(x2)),
										env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) <= math.Float64frombits(x2),
										env.stackTopAsUint32() == 1)
								}
							case wazeroir.OperationKindLt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) < int32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) < uint32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) < int64(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 < x2, env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) < math.Float32frombits(uint32(x2)),
										env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) < math.Float64frombits(x2),
										env.stackTopAsUint32() == 1)
								}
							case wazeroir.OperationKindGe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) >= int32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) >= uint32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) >= int64(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 >= x2, env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) >= math.Float32frombits(uint32(x2)),
										env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) >= math.Float64frombits(x2),
										env.stackTopAsUint32() == 1)
								}
							case wazeroir.OperationKindGt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) > int32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) > uint32(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) > int64(x2), env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 > x2, env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) > math.Float32frombits(uint32(x2)),
										env.stackTopAsUint32() == 1)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) > math.Float64frombits(x2),
										env.stackTopAsUint32() == 1)
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

							// Release the value to the memory stack again to verify the operation, and then return.
							compiler.releaseRegisterToStack(resultLocation)
							compiler.returnFunction()

							// Generate the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)

							// Run code.
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
				pickTargetLocation := compiler.locationStack.pushValueOnStack()
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
				pickTargetLocation := compiler.locationStack.pushValueOnStack()
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
			_ = compiler.locationStack.pushValueOnStack()
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

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
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
