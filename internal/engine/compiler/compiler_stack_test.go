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
			env := newCompilerEnvironment()

			// Compile code.
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			s := &valueLocationStack{
				sp:            tc.stackPointer,
				stack:         make([]*valueLocation, tc.stackPointer),
				usedRegisters: map[asm.Register]struct{}{},
			}
			// Peek must be non-nil. Otherwise, compileConst* would fail.
			s.stack[s.sp-1] = &valueLocation{}
			compiler.setValueLocationStack(s)

			if tc.isFloat {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
			}
			require.NoError(t, err)
			// Release the register allocated value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(s.peek())
			compiler.compileExitFromNativeCode(compilerCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack.
			env.callEngine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.exec(code)

			// Compiler status must be returned and stack pointer must end up the specified one.
			require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

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
			env := newCompilerEnvironment()

			// Compile code.
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			compiler.valueLocationStack().sp = tc.stackPointer
			compiler.valueLocationStack().stack = make([]*valueLocation, tc.stackPointer)

			// Record that that top value is on top.
			require.Zero(t, len(compiler.valueLocationStack().usedRegisters))
			loc := compiler.valueLocationStack().pushValueLocationOnStack()
			if tc.isFloat {
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			} else {
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			}
			// At this point the value must be recorded as being on stack.
			require.True(t, loc.onStack())

			// Release the stack-allocated value to register.
			err = compiler.compileEnsureOnGeneralPurposeRegister(loc)
			require.NoError(t, err)
			require.Equal(t, 1, len(compiler.valueLocationStack().usedRegisters))
			require.True(t, loc.onRegister())

			// To verify the behavior, increment the value on the register.
			if tc.isFloat {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			}

			// Release the value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(loc)
			require.NoError(t, err)
			compiler.compileExitFromNativeCode(compilerCallStatusCodeReturned)
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack, and place the original value.
			env.callEngine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.stack()[tc.stackPointer] = val
			env.exec(code)

			// Compiler status must be returned and stack pointer must end up the specified one.
			require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val)+1, env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val)+1, env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compilePick(t *testing.T) {
	const pickTargetValue uint64 = 12345
	op := &wazeroir.OperationPick{Depth: 1}

	for _, tc := range []struct {
		name                                      string
		pickTargetSetupFunc                       func(compiler compilerImpl, ce *callEngine) error
		isPickTargetFloat, isPickTargetOnRegister bool
	}{
		{
			name: "float on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(pickTargetValue)})
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: true,
		},
		{
			name: "int on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstI64(&wazeroir.OperationConstI64{Value: pickTargetValue})
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: true,
		},
		{
			name: "float on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.valueLocationStack().pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeFloat)
				ce.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: false,
		},
		{
			name: "int on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.valueLocationStack().pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeInt)
				ce.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the stack before picking.
			err = tc.pickTargetSetupFunc(compiler, env.callEngine())
			require.NoError(t, err)
			pickTargetLocation := compiler.valueLocationStack().peek()

			// Push the unused median value.
			_ = compiler.valueLocationStack().pushValueLocationOnStack()
			require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

			// Now ready to compile Pick operation.
			err = compiler.compilePick(op)
			require.NoError(t, err)
			require.Equal(t, uint64(3), compiler.valueLocationStack().sp)

			pickedLocation := compiler.valueLocationStack().peek()
			require.True(t, pickedLocation.onRegister())
			require.Equal(t, pickTargetLocation.registerType(), pickedLocation.registerType())

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Compile and execute the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the returned status and stack pointer.
			require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
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
			compiler.valueLocationStack().pushValueLocationOnStack()
		}
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Depth: nil})
		require.NoError(t, err)

		// After the nil range drop, the stack must remain the same.
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
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
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
				require.NoError(t, err)
			} else {
				compiler.valueLocationStack().pushValueLocationOnStack()
			}
		}
		require.Equal(t, uint64(liveNum+dropTargetNum), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Depth: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)
		// Plus, the top value must stay on a register.
		top := compiler.valueLocationStack().peek()
		require.True(t, top.onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
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

		// Put existing contents except the top on stack
		for i := 0; i < total-1; i++ {
			loc := compiler.valueLocationStack().pushValueLocationOnStack()
			ce.valueStack[loc.stackPointer] = uint64(i) // Put the initial value.
		}

		// Place the top value.
		const expectedTopLiveValue = 100
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
		require.NoError(t, err)

		require.Equal(t, uint64(total), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Depth: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveTotal), compiler.valueLocationStack().sp)
		// Plus, the top value must stay on a register.
		require.True(t, compiler.valueLocationStack().peek().onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, compilerCallStatusCodeReturned, env.compilerStatus())
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
	for i, tc := range []struct {
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
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			for _, vals := range [][2]uint64{
				{1, 2}, {0, 1}, {1, 0},
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

					x1 := compiler.valueLocationStack().pushValueLocationOnStack()
					env.stack()[x1.stackPointer] = x1Value
					if tc.x1OnRegister {
						err = compiler.compileEnsureOnGeneralPurposeRegister(x1)
						require.NoError(t, err)
					}

					x2 := compiler.valueLocationStack().pushValueLocationOnStack()
					env.stack()[x2.stackPointer] = x2Value
					if tc.x2OnRegister {
						err = compiler.compileEnsureOnGeneralPurposeRegister(x2)
						require.NoError(t, err)
					}

					var c *valueLocation
					if tc.condlValueOnStack {
						c = compiler.valueLocationStack().pushValueLocationOnStack()
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
					} else if tc.condValueOnGPRegister {
						c = compiler.valueLocationStack().pushValueLocationOnStack()
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
						err = compiler.compileEnsureOnGeneralPurposeRegister(c)
						require.NoError(t, err)
					} else if tc.condValueOnCondRegister {
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						if tc.selectX1 {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
						}
						require.NoError(t, err)
					}

					// Now emit code for select.
					err = compiler.compileSelect()
					require.NoError(t, err)

					// x1 should be top of the stack.
					require.Equal(t, x1, compiler.valueLocationStack().peek())

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Run code.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the selected value.
					require.Equal(t, uint64(1), env.stackPointer())
					if tc.selectX1 {
						require.Equal(t, env.stack()[x1.stackPointer], uint64(x1Value))
					} else {
						require.Equal(t, env.stack()[x1.stackPointer], uint64(x2Value))
					}
				})
			}
		})
	}
}

func TestCompiler_compileSwap(t *testing.T) {
	var x1Value, x2Value int64 = 100, 200
	for i, tc := range []struct {
		x1OnConditionalRegister, x1OnRegister, x2OnRegister bool
	}{
		{x1OnRegister: true, x2OnRegister: true},
		{x1OnRegister: true, x2OnRegister: false},
		{x1OnRegister: false, x2OnRegister: true},
		{x1OnRegister: false, x2OnRegister: false},
		// x1 on conditional register
		{x1OnConditionalRegister: true, x2OnRegister: false},
		{x1OnConditionalRegister: true, x2OnRegister: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			x2 := compiler.valueLocationStack().pushValueLocationOnStack()
			env.stack()[x2.stackPointer] = uint64(x2Value)
			if tc.x2OnRegister {
				err = compiler.compileEnsureOnGeneralPurposeRegister(x2)
				require.NoError(t, err)
			}

			_ = compiler.valueLocationStack().pushValueLocationOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := compiler.valueLocationStack().pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = uint64(x1Value)
				err = compiler.compileEnsureOnGeneralPurposeRegister(x1)
				require.NoError(t, err)
			} else if !tc.x1OnConditionalRegister {
				x1 := compiler.valueLocationStack().pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = uint64(x1Value)
			} else {
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
				x1Value = 1
			}

			// Swap x1 and x2.
			err = compiler.compileSwap(&wazeroir.OperationSwap{Depth: 2})
			require.NoError(t, err)

			require.NoError(t, compiler.compileReturnFunction())

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, uint64(3), env.stackPointer())
			// Check values are swapped.
			require.Equal(t, uint64(x1Value), env.stack()[0])
			require.Equal(t, uint64(x2Value), env.stack()[2])
		})
	}
}
