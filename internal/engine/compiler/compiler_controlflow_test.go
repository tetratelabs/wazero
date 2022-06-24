package compiler

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileHostFunction(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, nil)

	err := compiler.compileHostFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// On the return, the code must exit with the host call status.
	require.Equal(t, nativeCallStatusCodeCallHostFunction, env.compilerStatus())

	// Re-enter the return address.
	nativecall(env.callFrameStackPeek().returnAddress,
		uintptr(unsafe.Pointer(env.callEngine())),
		uintptr(unsafe.Pointer(env.module())),
	)

	// After that, the code must exit with returned status.
	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
}

func TestCompiler_compileLabel(t *testing.T) {
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	for _, expectSkip := range []bool{false, true} {
		expectSkip := expectSkip
		t.Run(fmt.Sprintf("expect skip=%v", expectSkip), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)

			if expectSkip {
				// If the initial stack is not set, compileLabel must return skip=true.
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.True(t, actual)
			} else {
				err := compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: label}})
				require.NoError(t, err)
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.False(t, actual)
			}
		})
	}
}

func TestCompiler_compileBrIf(t *testing.T) {
	unreachableStatus, thenLabelExitStatus, elseLabelExitStatus :=
		nativeCallStatusCodeUnreachable, nativeCallStatusCodeUnreachable+1, nativeCallStatusCodeUnreachable+2
	thenBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}}}
	elseBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 2}}}

	tests := []struct {
		name      string
		setupFunc func(t *testing.T, compiler compilerImpl, shouldGoElse bool)
	}{
		{
			name: "cond on register",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				val := uint32(1)
				if shouldGoElse {
					val = 0
				}
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: val})
				require.NoError(t, err)
			},
		},
		{
			name: "LS",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := float32(1), float32(2)
				if shouldGoElse {
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
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(1)
				if shouldGoElse {
					x2++
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
		{
			name: "NE",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2 = x1
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			for _, shouldGoToElse := range []bool{false, true} {
				shouldGoToElse := shouldGoToElse
				t.Run(fmt.Sprintf("should_goto_else=%v", shouldGoToElse), func(t *testing.T) {
					env := newCompilerEnvironment()
					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					tc.setupFunc(t, compiler, shouldGoToElse)
					require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)

					err = compiler.compileBrIf(&wazeroir.OperationBrIf{Then: thenBranchTarget, Else: elseBranchTarget})
					require.NoError(t, err)
					compiler.compileExitFromNativeCode(unreachableStatus)

					// Emit code for .then label.
					skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: thenBranchTarget.Target.Label})
					require.False(t, skip)
					compiler.compileExitFromNativeCode(thenLabelExitStatus)

					// Emit code for .else label.
					skip = compiler.compileLabel(&wazeroir.OperationLabel{Label: elseBranchTarget.Target.Label})
					require.False(t, skip)
					compiler.compileExitFromNativeCode(elseLabelExitStatus)

					code, _, err := compiler.compile()
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
					require.NotEqual(t, unreachableStatus, env.compilerStatus())
					if shouldGoToElse {
						require.Equal(t, elseLabelExitStatus, env.compilerStatus())
					} else {
						require.Equal(t, thenLabelExitStatus, env.compilerStatus())
					}
				})
			}
		})
	}
}

func TestCompiler_compileBrTable(t *testing.T) {
	requireRunAndExpectedValueReturned := func(t *testing.T, env *compilerEnv, c compilerImpl, expValue uint32) {
		// Emit code for each label which returns the frame ID.
		for returnValue := uint32(0); returnValue < 7; returnValue++ {
			label := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			err := c.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: label}})
			require.NoError(t, err)
			_ = c.compileLabel(&wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(&wazeroir.OperationConstI32{Value: label.FrameID})
			err = c.compileReturnFunction()
			require.NoError(t, err)
		}

		// Generate the code under test and run.
		code, _, err := c.compile()
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

	tests := []struct {
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
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.index)})
			require.NoError(t, err)

			err = compiler.compileBrTable(tc.o)
			require.NoError(t, err)

			require.Zero(t, len(compiler.runtimeValueLocationStack().usedRegisters))

			requireRunAndExpectedValueReturned(t, env, compiler, tc.expectedValue)
		})
	}
}

func requirePushTwoInt32Consts(t *testing.T, x1, x2 uint32, compiler compilerImpl) {
	err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2})
	require.NoError(t, err)
}

func requirePushTwoFloat32Consts(t *testing.T, x1, x2 float32, compiler compilerImpl) {
	err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x2})
	require.NoError(t, err)
}

func TestCompiler_compileBr(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Branch into nil label is interpreted as return. See BranchTarget.IsReturnTarget
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: nil}})
		require.NoError(t, err)

		// Compile and execute the code under test.
		// Note: we don't invoke "compiler.return()" as the code emitted by compilerBr is enough to exit.
		code, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
	})
	t.Run("back-and-forth br", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Emit the forward br, meaning that handle Br instruction where the target label hasn't been compiled yet.
		forwardLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: forwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with Unreachable status.
		compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)
		require.NoError(t, err)

		exitLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: exitLabel}})
		require.NoError(t, err)

		// Emit code for the exitLabel.
		skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: exitLabel})
		require.False(t, skip)
		compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)
		require.NoError(t, err)

		// Emit code for the forwardLabel.
		skip = compiler.compileLabel(&wazeroir.OperationLabel{Label: forwardLabel})
		require.False(t, skip)
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: exitLabel}})
		require.NoError(t, err)

		code, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		//    ... code from compilePreamble()
		//    br .forwardLabel
		//    exit nativeCallStatusCodeUnreachable  // must not be reached
		//    br .exitLabel                      // must not be reached
		// .exitLabel:
		//    exit nativeCallStatusCodeReturned
		// .forwardLabel:
		//    br .exitLabel
		//
		// Therefore, if we start executing from the top, we must end up exiting nativeCallStatusCodeReturned.
		env.exec(code)
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
	})
}

func TestCompiler_compileCallIndirect(t *testing.T) {
	t.Run("out of bounds", func(t *testing.T) {
		env := newCompilerEnvironment()
		env.addTable(&wasm.TableInstance{References: make([]wasm.Reference, 10)})
		compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
			Signature: &wasm.FunctionType{},
			Types:     []*wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}

		// Place the offset value.
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
		require.NoError(t, err)

		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)

		// Generate the code under test and run.
		code, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
	})

	t.Run("uninitialized", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
			Signature: &wasm.FunctionType{},
			Types:     []*wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}

		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]wasm.Reference, 10)
		env.addTable(&wasm.TableInstance{References: table})
		env.module().TypeIDs = make([]wasm.FunctionTypeID, 10)

		// Place the offset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)
		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
	})

	t.Run("type not match", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
			Signature: &wasm.FunctionType{},
			Types:     []*wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		env.module().TypeIDs = []wasm.FunctionTypeID{1000}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]wasm.Reference, 10)
		env.addTable(&wasm.TableInstance{References: table})

		cf := &function{source: &wasm.FunctionInstance{TypeID: 50}}
		table[0] = uintptr(unsafe.Pointer(cf))

		// Place the offset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)

		// Now emit the code.
		require.NoError(t, compiler.compileCallIndirect(targetOperation))

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeTypeMismatchOnIndirectCall.String(), env.compilerStatus().String())
	})

	t.Run("ok", func(t *testing.T) {
		for _, growCallFrameStack := range []bool{false} {
			growCallFrameStack := growCallFrameStack
			t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
				targetType := &wasm.FunctionType{
					Params:  []wasm.ValueType{},
					Results: []wasm.ValueType{wasm.ValueTypeI32}}
				targetTypeID := wasm.FunctionTypeID(10) // Arbitrary number is fine for testing.
				operation := &wazeroir.OperationCallIndirect{TypeIndex: 0}

				table := make([]wasm.Reference, 10)
				env := newCompilerEnvironment()
				env.addTable(&wasm.TableInstance{References: table})

				// Ensure that the module instance has the type information for targetOperation.TypeIndex,
				// and the typeID  matches the table[targetOffset]'s type ID.
				env.module().TypeIDs = make([]wasm.FunctionTypeID, 100)
				env.module().TypeIDs[operation.TypeIndex] = targetTypeID
				env.module().Engine = &moduleEngine{functions: []*function{}}

				me := env.moduleEngine()
				for i := 0; i < len(table); i++ {
					// First we create the call target function with function address = i,
					// and it returns one value.
					expectedReturnValue := uint32(i * 1000)

					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)
					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
					require.NoError(t, err)
					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					c, _, err := compiler.compile()
					require.NoError(t, err)

					f := &function{
						parent:                &code{codeSegment: c},
						codeInitialAddress:    uintptr(unsafe.Pointer(&c[0])),
						moduleInstanceAddress: uintptr(unsafe.Pointer(env.moduleInstance)),
						source: &wasm.FunctionInstance{
							TypeID: targetTypeID,
						},
					}
					me.functions = append(me.functions, f)
					table[i] = uintptr(unsafe.Pointer(f))
				}

				for i := 1; i < len(table); i++ {
					expectedReturnValue := uint32(i * 1000)
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						if growCallFrameStack {
							env.setCallFrameStackPointerLen(1)
						}

						compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
							Signature: targetType,
							Types:     []*wasm.FunctionType{targetType},
							HasTable:  true,
						})
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Place the offset value. Here we try calling a function of functionaddr == table[i].FunctionIndex.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(i)})
						require.NoError(t, err)

						// At this point, we should have one item (offset value) on the stack.
						require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)

						require.NoError(t, compiler.compileCallIndirect(operation))

						// At this point, we consumed the offset value, but the function returns one value,
						// so the stack pointer results in the same.
						require.Equal(t, uint64(1), compiler.runtimeValueLocationStack().sp)

						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						// Generate the code under test and run.
						code, _, err := compiler.compile()
						require.NoError(t, err)
						env.exec(code)

						if growCallFrameStack {
							// If the call frame stack pointer equals the length of call frame stack length,
							// we have to call the builtin function to grow the slice.
							require.Equal(t, nativeCallStatusCodeCallBuiltInFunction, env.compilerStatus())
							require.Equal(t, builtinFunctionIndexGrowCallFrameStack, env.builtinFunctionCallAddress())

							// Grow the callFrame stack, and exec again from the return address.
							ce := env.callEngine()
							ce.builtinFunctionGrowCallFrameStack()
							nativecall(
								env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(ce)),
								uintptr(unsafe.Pointer(env.module())),
							)
						}

						require.Equal(t, nativeCallStatusCodeReturned.String(), env.compilerStatus().String())
						require.Equal(t, uint64(1), env.stackPointer())
						require.Equal(t, expectedReturnValue, uint32(env.ce.popValue()))
					})
				}
			})
		}
	})
}

// TestCompiler_callIndirect_largeTypeIndex ensures that non-trivial large type index works well during call_indirect.
// Note: any index larger than 8-bit range is considered as large for arm64 compiler.
func TestCompiler_callIndirect_largeTypeIndex(t *testing.T) {
	env := newCompilerEnvironment()
	table := make([]wasm.Reference, 1)
	env.addTable(&wasm.TableInstance{References: table})
	// Ensure that the module instance has the type information for targetOperation.TypeIndex,
	// and the typeID  matches the table[targetOffset]'s type ID.
	const typeIndex, typeID = 12345, 0
	operation := &wazeroir.OperationCallIndirect{TypeIndex: typeIndex}
	env.module().TypeIDs = make([]wasm.FunctionTypeID, typeIndex+1)
	env.module().TypeIDs[typeIndex] = typeID
	env.module().Engine = &moduleEngine{functions: []*function{}}

	types := make([]*wasm.FunctionType, typeIndex+1)
	types[typeIndex] = &wasm.FunctionType{}

	me := env.moduleEngine()
	{ // Compiling call target.
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		c, _, err := compiler.compile()
		require.NoError(t, err)

		f := &function{
			parent:                &code{codeSegment: c},
			codeInitialAddress:    uintptr(unsafe.Pointer(&c[0])),
			moduleInstanceAddress: uintptr(unsafe.Pointer(env.moduleInstance)),
			source:                &wasm.FunctionInstance{TypeID: 0},
		}
		me.functions = append(me.functions, f)
		table[0] = uintptr(unsafe.Pointer(f))
	}

	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Types:     types,
		HasTable:  true,
	})
	err := compiler.compilePreamble()
	require.NoError(t, err)

	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
	require.NoError(t, err)

	require.NoError(t, compiler.compileCallIndirect(operation))

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test and run.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)
}

func TestCompiler_compileCall(t *testing.T) {
	for _, growCallFrameStack := range []bool{false, true} {
		growCallFrameStack := growCallFrameStack
		t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
			env := newCompilerEnvironment()
			me := env.moduleEngine()
			expectedValue := uint32(0)

			if growCallFrameStack {
				env.setCallFrameStackPointerLen(1)
			}

			// Emit the call target function.
			const numCalls = 3
			targetFunctionType := &wasm.FunctionType{
				Params:           []wasm.ValueType{wasm.ValueTypeI32},
				Results:          []wasm.ValueType{wasm.ValueTypeI32},
				ParamNumInUint64: 1, ResultNumInUint64: 1,
			}
			for i := 0; i < numCalls; i++ {
				// Each function takes one argument, adds the value with 100 + i and returns the result.
				addTargetValue := uint32(100 + i)
				expectedValue += addTargetValue

				compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{Signature: targetFunctionType})

				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(addTargetValue)})
				require.NoError(t, err)
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)

				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				c, _, err := compiler.compile()
				require.NoError(t, err)
				index := wasm.Index(i)
				me.functions = append(me.functions, &function{
					parent:                &code{codeSegment: c},
					codeInitialAddress:    uintptr(unsafe.Pointer(&c[0])),
					moduleInstanceAddress: uintptr(unsafe.Pointer(env.moduleInstance)),
				})
				env.module().Functions = append(env.module().Functions,
					&wasm.FunctionInstance{Type: targetFunctionType, Idx: index})
			}

			// Now we start building the caller's code.
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				Signature: &wasm.FunctionType{},
				Functions: make([]uint32, numCalls),
				Types:     []*wasm.FunctionType{targetFunctionType},
			})

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

			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			if growCallFrameStack {
				// If the call frame stack pointer equals the length of call frame stack length,
				// we have to call the builtin function to grow the slice.
				require.Equal(t, nativeCallStatusCodeCallBuiltInFunction, env.compilerStatus())
				require.Equal(t, builtinFunctionIndexGrowCallFrameStack, env.builtinFunctionCallAddress())

				// Grow the callFrame stack, and exec again from the return address.
				ce := env.callEngine()
				ce.builtinFunctionGrowCallFrameStack()
				nativecall(
					env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(ce)),
					uintptr(unsafe.Pointer(env.module())),
				)
			}

			// Check status and returned values.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, uint64(2), env.stackPointer()) // Must be 2 (dummy value + the calculation results)
			require.Equal(t, uint64(0), env.stackBasePointer())
			require.Equal(t, expectedValue, env.stackTopAsUint32())
		})
	}
}

func TestCompiler_returnFunction(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		env := newCompilerEnvironment()

		// Compile code.
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)

		// Compiler status must be returned.
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
		// Plus, the call frame stack pointer must be zero after return.
		require.Equal(t, uint64(0), env.callFrameStackPointer())
	})
	t.Run("deep call stack", func(t *testing.T) {
		env := newCompilerEnvironment()
		moduleEngine := env.moduleEngine()
		ce := env.callEngine()

		// Push the call frames.
		const callFrameNums = 10
		stackPointerToExpectedValue := map[uint64]uint32{}
		for funcIndex := wasm.Index(0); funcIndex < callFrameNums; funcIndex++ {
			// Each function pushes its funcaddr and soon returns.
			compiler := env.requireNewCompiler(t, newCompiler, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Push its functionIndex.
			expValue := uint32(funcIndex)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			c, _, err := compiler.compile()
			require.NoError(t, err)

			// Compiles and adds to the engine.
			f := &function{
				parent:                &code{codeSegment: c},
				codeInitialAddress:    uintptr(unsafe.Pointer(&c[0])),
				moduleInstanceAddress: uintptr(unsafe.Pointer(env.moduleInstance)),
			}
			moduleEngine.functions = append(moduleEngine.functions, f)

			// Pushes the frame whose return address equals the beginning of the function just compiled.
			frame := callFrame{
				// Set the return address to the beginning of the function so that we can execute the constI32 above.
				returnAddress: f.codeInitialAddress,
				// Note: return stack base pointer is set to funcaddr*5 and this is where the const should be pushed.
				returnStackBasePointer: uint64(funcIndex) * 5,
				function:               f,
			}
			ce.callFrameStack[ce.globalContext.callFrameStackPointer] = frame
			ce.globalContext.callFrameStackPointer++
			stackPointerToExpectedValue[frame.returnStackBasePointer] = expValue
		}

		require.Equal(t, uint64(callFrameNums), env.callFrameStackPointer())

		// Run code from the top frame.
		env.exec(ce.callFrameTop().function.parent.codeSegment)

		// Check the exit status and the values on stack.
		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
		for pos, exp := range stackPointerToExpectedValue {
			require.Equal(t, exp, uint32(env.stack()[pos]))
		}
	})
}
