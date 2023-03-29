package compiler

import (
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_releaseRegisterToStack(t *testing.T) {
	const val = 10000
	tests := []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			// Compile code.
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the location stack so that we push the const on the specified height.
			s := runtimeValueLocationStack{
				sp:                                tc.stackPointer,
				stack:                             make([]runtimeValueLocation, tc.stackPointer),
				usedRegisters:                     map[asm.Register]struct{}{},
				unreservedVectorRegisters:         unreservedVectorRegisters,
				unreservedGeneralPurposeRegisters: unreservedGeneralPurposeRegisters,
			}
			// Peek must be non-nil. Otherwise, compileConst* would fail.
			compiler.setRuntimeValueLocationStack(s)

			if tc.isFloat {
				err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
			} else {
				err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: val})
			}
			require.NoError(t, err)
			// Release the register allocated value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(compiler.runtimeValueLocationStack().peek())
			compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack.
			env.callEngine().builtinFunctionGrowStack(tc.stackPointer)
			env.exec(code)

			// Compiler status must be returned and stack pointer must end up the specified one.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, tc.stackPointer+1, env.ce.stackPointer)

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val), env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val), env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compileLoadValueOnStackToRegister(t *testing.T) {
	const val = 123
	tests := []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			// Compile code.
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			compiler.runtimeValueLocationStack().sp = tc.stackPointer
			compiler.runtimeValueLocationStack().stack = make([]runtimeValueLocation, tc.stackPointer)

			require.Zero(t, len(compiler.runtimeValueLocationStack().usedRegisters))
			loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			if tc.isFloat {
				loc.valueType = runtimeValueTypeF64
			} else {
				loc.valueType = runtimeValueTypeI64
			}
			// At this point the value must be recorded as being on stack.
			require.True(t, loc.onStack())

			// Release the stack-allocated value to register.
			err = compiler.compileEnsureOnRegister(loc)
			require.NoError(t, err)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
			require.True(t, loc.onRegister())

			// To verify the behavior, increment the value on the register.
			if tc.isFloat {
				err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeF64))
				require.NoError(t, err)
			} else {
				err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeI64))
				require.NoError(t, err)
			}

			// Release the value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(loc)
			require.NoError(t, err)
			compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)
			require.NoError(t, err)

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack, and place the original value.
			env.callEngine().builtinFunctionGrowStack(tc.stackPointer)
			env.stack()[tc.stackPointer] = val
			env.exec(code)

			// Compiler status must be returned and stack pointer must end up the specified one.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, tc.stackPointer+1, env.ce.stackPointer)

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val)+1, env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val)+1, env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compilePick_v128(t *testing.T) {
	const pickTargetLo, pickTargetHi uint64 = 12345, 6789

	op := wazeroir.OperationPick{Depth: 2, IsTargetVector: true}
	tests := []struct {
		name                   string
		isPickTargetOnRegister bool
	}{
		{name: "target on register", isPickTargetOnRegister: false},
		{name: "target on stack", isPickTargetOnRegister: true},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the stack before picking.
			if tc.isPickTargetOnRegister {
				err = compiler.compileV128Const(wazeroir.OperationV128Const{
					Lo: pickTargetLo, Hi: pickTargetHi,
				})
				require.NoError(t, err)
			} else {
				lo := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // lo
				lo.valueType = runtimeValueTypeV128Lo
				env.stack()[lo.stackPointer] = pickTargetLo
				hi := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // hi
				hi.valueType = runtimeValueTypeV128Hi
				env.stack()[hi.stackPointer] = pickTargetHi
			}

			// Push the unused median value.
			_ = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			requireRuntimeLocationStackPointerEqual(t, uint64(3), compiler)

			// Now ready to compile Pick operation.
			err = compiler.compilePick(op)
			require.NoError(t, err)
			requireRuntimeLocationStackPointerEqual(t, uint64(5), compiler)

			hiLoc := compiler.runtimeValueLocationStack().peek()
			loLoc := compiler.runtimeValueLocationStack().stack[hiLoc.stackPointer-1]
			require.True(t, hiLoc.onRegister())
			require.Equal(t, runtimeValueTypeV128Hi, hiLoc.valueType)
			require.Equal(t, runtimeValueTypeV128Lo, loLoc.valueType)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Compile and execute the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the returned status and stack pointer.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, uint64(5), env.stackPointer())

			// Verify the top value is the picked one and the pick target's value stays the same.
			lo, hi := env.stackTopAsV128()
			require.Equal(t, pickTargetLo, lo)
			require.Equal(t, pickTargetHi, hi)
			require.Equal(t, pickTargetLo, env.stack()[loLoc.stackPointer])
			require.Equal(t, pickTargetHi, env.stack()[hiLoc.stackPointer])
		})
	}
}

func TestCompiler_compilePick(t *testing.T) {
	const pickTargetValue uint64 = 12345
	op := wazeroir.OperationPick{Depth: 1}
	tests := []struct {
		name                                      string
		pickTargetSetupFunc                       func(compiler compilerImpl, ce *callEngine) error
		isPickTargetFloat, isPickTargetOnRegister bool
	}{
		{
			name: "float on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(pickTargetValue)})
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: true,
		},
		{
			name: "int on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstI64(wazeroir.OperationConstI64{Value: pickTargetValue})
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: true,
		},
		{
			name: "float on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
				pickTargetLocation.valueType = runtimeValueTypeF64
				ce.stack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: false,
		},
		{
			name: "int on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
				pickTargetLocation.valueType = runtimeValueTypeI64
				ce.stack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: false,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the stack before picking.
			err = tc.pickTargetSetupFunc(compiler, env.callEngine())
			require.NoError(t, err)
			pickTargetLocation := compiler.runtimeValueLocationStack().peek()

			// Push the unused median value.
			_ = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)

			// Now ready to compile Pick operation.
			err = compiler.compilePick(op)
			require.NoError(t, err)
			requireRuntimeLocationStackPointerEqual(t, uint64(3), compiler)

			pickedLocation := compiler.runtimeValueLocationStack().peek()
			require.True(t, pickedLocation.onRegister())
			require.Equal(t, pickTargetLocation.getRegisterType(), pickedLocation.getRegisterType())

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Compile and execute the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the returned status and stack pointer.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
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

func TestCompiler_compileDrop(t *testing.T) {
	t.Run("range nil", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		liveNum := 10
		for i := 0; i < liveNum; i++ {
			compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
		}
		requireRuntimeLocationStackPointerEqual(t, uint64(liveNum), compiler)

		err = compiler.compileDrop(wazeroir.OperationDrop{Depth: nil})
		require.NoError(t, err)

		// After the nil range drop, the stack must remain the same.
		requireRuntimeLocationStackPointerEqual(t, uint64(liveNum), compiler)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
	})
	t.Run("start top", func(t *testing.T) {
		r := &wazeroir.InclusiveRange{Start: 0, End: 2}
		dropTargetNum := r.End - r.Start + 1 // +1 as the range is inclusive!
		liveNum := 5

		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		const expectedTopLiveValue = 100
		for i := 0; i < liveNum+dropTargetNum; i++ {
			if i == liveNum-1 {
				err := compiler.compileConstI64(wazeroir.OperationConstI64{Value: expectedTopLiveValue})
				require.NoError(t, err)
			} else {
				compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			}
		}
		requireRuntimeLocationStackPointerEqual(t, uint64(liveNum+dropTargetNum), compiler)

		err = compiler.compileDrop(wazeroir.OperationDrop{Depth: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		requireRuntimeLocationStackPointerEqual(t, uint64(liveNum), compiler)
		// Plus, the top value must stay on a register.
		top := compiler.runtimeValueLocationStack().peek()
		require.True(t, top.onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
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

		env := newCompilerEnvironment()
		ce := env.callEngine()
		compiler := env.requireNewCompiler(t, newCompiler, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// We don't need call frame in this test case, so simply pop them out!
		for i := 0; i < callFrameDataSizeInUint64; i++ {
			compiler.runtimeValueLocationStack().pop()
		}

		// Put existing contents except the top on stack
		for i := 0; i < total-1; i++ {
			loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			loc.valueType = runtimeValueTypeI32
			ce.stack[loc.stackPointer] = uint64(i) // Put the initial value.
		}

		// Place the top value.
		const expectedTopLiveValue = 100
		err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: expectedTopLiveValue})
		require.NoError(t, err)

		require.Equal(t, uint64(total), compiler.runtimeValueLocationStack().sp)

		err = compiler.compileDrop(wazeroir.OperationDrop{Depth: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveTotal), compiler.runtimeValueLocationStack().sp)
		// Plus, the top value must stay on a register.
		require.True(t, compiler.runtimeValueLocationStack().peek().onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
		require.Equal(t, uint64(liveTotal), env.ce.stackPointer)

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

func TestCompiler_compileSelect(t *testing.T) {
	// There are mainly 8 cases we have to test:
	// - [x1 = reg, x2 = reg] select x1
	// - [x1 = reg, x2 = reg] select x2
	// - [x1 = reg, x2 = stack] select x1
	// - [x1 = reg, x2 = stack] select x2
	// - [x1 = stack, x2 = reg] select x1
	// - [x1 = stack, x2 = reg] select x2
	// - [x1 = stack, x2 = stack] select x1
	// - [x1 = stack, x2 = stack] select x2
	// And for each case, we have to test with
	// three conditional value location: stack, gp register, conditional register.
	// So in total we have 24 cases.
	tests := []struct {
		x1OnRegister, x2OnRegister                                        bool
		selectX1                                                          bool
		condlValueOnStack, condValueOnGPRegister, condValueOnCondRegister bool
	}{
		// Conditional value on stack.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		// Conditional value on register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		// Conditional value on conditional register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			for _, vals := range [][2]uint64{
				{1, 2},
				{0, 1},
				{1, 0},
				{math.Float64bits(-1), math.Float64bits(-1)},
				{math.Float64bits(-1), math.Float64bits(1)},
				{math.Float64bits(1), math.Float64bits(-1)},
			} {
				x1Value, x2Value := vals[0], vals[1]
				t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", vals[0], vals[1]), func(t *testing.T) {
					env := newCompilerEnvironment()
					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					x1 := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
					x1.valueType = runtimeValueTypeI64
					env.stack()[x1.stackPointer] = x1Value
					if tc.x1OnRegister {
						err = compiler.compileEnsureOnRegister(x1)
						require.NoError(t, err)
					}

					x2 := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
					x2.valueType = runtimeValueTypeI64
					env.stack()[x2.stackPointer] = x2Value
					if tc.x2OnRegister {
						err = compiler.compileEnsureOnRegister(x2)
						require.NoError(t, err)
					}

					var c *runtimeValueLocation
					if tc.condlValueOnStack {
						c = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
						c.valueType = runtimeValueTypeI32
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
					} else if tc.condValueOnGPRegister {
						c = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
						c.valueType = runtimeValueTypeI32
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
						err = compiler.compileEnsureOnRegister(c)
						require.NoError(t, err)
					} else if tc.condValueOnCondRegister {
						err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						if tc.selectX1 {
							err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI32))
						} else {
							err = compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeI32))
						}
						require.NoError(t, err)
					}

					// Now emit code for select.
					err = compiler.compileSelect(wazeroir.OperationSelect{})
					require.NoError(t, err)

					// x1 should be top of the stack.
					require.Equal(t, x1, compiler.runtimeValueLocationStack().peek())

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Run code.
					code, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the selected value.
					require.Equal(t, uint64(1), env.stackPointer())
					if tc.selectX1 {
						require.Equal(t, env.stack()[x1.stackPointer], x1Value)
					} else {
						require.Equal(t, env.stack()[x1.stackPointer], x2Value)
					}
				})
			}
		})
	}
}

func TestCompiler_compileSwap_v128(t *testing.T) {
	const x1Lo, x1Hi uint64 = 100000, 200000
	const x2Lo, x2Hi uint64 = 1, 2

	tests := []struct {
		x1OnRegister, x2OnRegister bool
	}{
		{x1OnRegister: true, x2OnRegister: true},
		{x1OnRegister: true, x2OnRegister: false},
		{x1OnRegister: false, x2OnRegister: true},
		{x1OnRegister: false, x2OnRegister: false},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("x1_register=%v, x2_register=%v", tc.x1OnRegister, tc.x2OnRegister), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			if tc.x1OnRegister {
				err = compiler.compileV128Const(wazeroir.OperationV128Const{Lo: x1Lo, Hi: x1Hi})
				require.NoError(t, err)
			} else {
				lo := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // lo
				lo.valueType = runtimeValueTypeV128Lo
				env.stack()[lo.stackPointer] = x1Lo
				hi := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // hi
				hi.valueType = runtimeValueTypeV128Hi
				env.stack()[hi.stackPointer] = x1Hi
			}

			_ = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // Dummy value!

			if tc.x2OnRegister {
				err = compiler.compileV128Const(wazeroir.OperationV128Const{Lo: x2Lo, Hi: x2Hi})
				require.NoError(t, err)
			} else {
				lo := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // lo
				lo.valueType = runtimeValueTypeV128Lo
				env.stack()[lo.stackPointer] = x2Lo
				hi := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // hi
				hi.valueType = runtimeValueTypeV128Hi
				env.stack()[hi.stackPointer] = x2Hi
			}

			// Swap x1 and x2.
			err = compiler.compileSet(wazeroir.OperationSet{Depth: 4, IsTargetVector: true})
			require.NoError(t, err)

			require.NoError(t, compiler.compileReturnFunction())

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, uint64(3), env.stackPointer())

			// The first variable is above the call frame.
			st := env.stack()
			require.Equal(t, x2Lo, st[callFrameDataSizeInUint64])
			require.Equal(t, x2Hi, st[callFrameDataSizeInUint64+1])
		})
	}
}

func TestCompiler_compileSet(t *testing.T) {
	var x1Value, x2Value int64 = 100, 200
	tests := []struct {
		x1OnConditionalRegister, x1OnRegister, x2OnRegister bool
	}{
		{x1OnRegister: true, x2OnRegister: true},
		{x1OnRegister: true, x2OnRegister: false},
		{x1OnRegister: false, x2OnRegister: true},
		{x1OnRegister: false, x2OnRegister: false},
		// x1 on conditional register
		{x1OnConditionalRegister: true, x2OnRegister: false},
		{x1OnConditionalRegister: true, x2OnRegister: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			x2 := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			x2.valueType = runtimeValueTypeI32
			env.stack()[x2.stackPointer] = uint64(x2Value)
			if tc.x2OnRegister {
				err = compiler.compileEnsureOnRegister(x2)
				require.NoError(t, err)
			}

			_ = compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
				x1.valueType = runtimeValueTypeI32
				env.stack()[x1.stackPointer] = uint64(x1Value)
				err = compiler.compileEnsureOnRegister(x1)
				require.NoError(t, err)
			} else if !tc.x1OnConditionalRegister {
				x1 := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
				x1.valueType = runtimeValueTypeI32
				env.stack()[x1.stackPointer] = uint64(x1Value)
			} else {
				err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI32))
				require.NoError(t, err)
				x1Value = 1
			}

			// Set x2 into the x1.
			err = compiler.compileSet(wazeroir.OperationSet{Depth: 2})
			require.NoError(t, err)

			require.NoError(t, compiler.compileReturnFunction())

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, uint64(2), env.stackPointer())
			// Check the value was set. Note that it is placed above the call frame.
			require.Equal(t, uint64(x1Value), env.stack()[callFrameDataSizeInUint64])
		})
	}
}
