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

	err := compiler.compileGoDefinedHostFunction()
	require.NoError(t, err)

	// Get the location of caller function's location stored in the stack, which depends on the type.
	// In this test, the host function has empty sig.
	_, _, callerFuncLoc := compiler.runtimeValueLocationStack().getCallFrameLocations(&wasm.FunctionType{})

	// Generate the machine code for the test.
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Set the caller's function which always exists in the real usecase.
	f := &function{moduleInstance: &wasm.ModuleInstance{}}
	env.stack()[callerFuncLoc.stackPointer] = uint64(uintptr(unsafe.Pointer(f)))
	env.exec(code)

	// On the return, the code must exit with the host call status.
	require.Equal(t, nativeCallStatusCodeCallGoHostFunction, env.compilerStatus())
	// Plus, the exitContext holds the caller's wasm.FunctionInstance.
	require.Equal(t, f.moduleInstance, env.ce.exitContext.callerModuleInstance)

	// Re-enter the return address.
	require.NotEqual(t, uintptr(0), uintptr(env.ce.returnAddress))
	nativecall(env.ce.returnAddress,
		uintptr(unsafe.Pointer(env.callEngine())),
		env.module(),
	)

	// After that, the code must exit with returned status.
	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
}

func TestCompiler_compileLabel(t *testing.T) {
	label := wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	for _, expectSkip := range []bool{false, true} {
		expectSkip := expectSkip
		t.Run(fmt.Sprintf("expect skip=%v", expectSkip), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)

			if expectSkip {
				// If the initial stack is not set, compileLabel must return skip=true.
				actual := compiler.compileLabel(wazeroir.OperationLabel{Label: label})
				require.True(t, actual)
			} else {
				err := compiler.compileBr(wazeroir.OperationBr{Target: label})
				require.NoError(t, err)
				actual := compiler.compileLabel(wazeroir.OperationLabel{Label: label})
				require.False(t, actual)
			}
		})
	}
}

func TestCompiler_compileBrIf(t *testing.T) {
	unreachableStatus, thenLabelExitStatus, elseLabelExitStatus := nativeCallStatusCodeUnreachable, nativeCallStatusCodeUnreachable+1, nativeCallStatusCodeUnreachable+2
	thenBranchTarget := wazeroir.BranchTargetDrop{Target: wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}}
	elseBranchTarget := wazeroir.BranchTargetDrop{Target: wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 2}}

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
				err := compiler.compileConstI32(wazeroir.OperationConstI32{Value: val})
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
				err := compiler.compileLe(wazeroir.NewOperationLe(wazeroir.SignedTypeUint32))
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
				err := compiler.compileLe(wazeroir.NewOperationLe(wazeroir.SignedTypeInt32))
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
				err := compiler.compileGe(wazeroir.NewOperationGe(wazeroir.SignedTypeUint32))
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
				err := compiler.compileGe(wazeroir.NewOperationGe(wazeroir.SignedTypeInt32))
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
				err := compiler.compileGt(wazeroir.NewOperationGt(wazeroir.SignedTypeUint32))
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
				err := compiler.compileGt(wazeroir.NewOperationGt(wazeroir.SignedTypeInt32))
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
				err := compiler.compileLt(wazeroir.NewOperationLt(wazeroir.SignedTypeUint32))
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
				err := compiler.compileLt(wazeroir.NewOperationLt(wazeroir.SignedTypeInt32))
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
				err := compiler.compileLt(wazeroir.NewOperationLt(wazeroir.SignedTypeFloat32))
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
				err := compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI32))
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
				err := compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeI32))
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
					requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)

					err = compiler.compileBrIf(wazeroir.OperationBrIf{Then: thenBranchTarget, Else: elseBranchTarget})
					require.NoError(t, err)
					compiler.compileExitFromNativeCode(unreachableStatus)

					// Emit code for .then label.
					skip := compiler.compileLabel(wazeroir.OperationLabel{Label: thenBranchTarget.Target})
					require.False(t, skip)
					compiler.compileExitFromNativeCode(thenLabelExitStatus)

					// Emit code for .else label.
					skip = compiler.compileLabel(wazeroir.OperationLabel{Label: elseBranchTarget.Target})
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
			label := wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			err := c.compileBr(wazeroir.OperationBr{Target: label})
			require.NoError(t, err)
			_ = c.compileLabel(wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(wazeroir.OperationConstI32{Value: label.FrameID})
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
		return &wazeroir.BranchTargetDrop{
			Target: wazeroir.Label{FrameID: frameid, Kind: wazeroir.LabelKindHeader},
		}
	}

	tests := []struct {
		name          string
		index         int64
		o             wazeroir.OperationBrTable
		expectedValue uint32
	}{
		{
			name:          "only default with index 0",
			o:             wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         0,
			expectedValue: 6,
		},
		{
			name:          "only default with index 100",
			o:             wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         100,
			expectedValue: 6,
		},
		{
			name: "select default with targets and good index",
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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
			o: wazeroir.OperationBrTable{
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

			err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(tc.index)})
			require.NoError(t, err)

			err = compiler.compileBrTable(tc.o)
			require.NoError(t, err)

			require.Zero(t, len(compiler.runtimeValueLocationStack().usedRegisters))

			requireRunAndExpectedValueReturned(t, env, compiler, tc.expectedValue)
		})
	}
}

func requirePushTwoInt32Consts(t *testing.T, x1, x2 uint32, compiler compilerImpl) {
	err := compiler.compileConstI32(wazeroir.OperationConstI32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: x2})
	require.NoError(t, err)
}

func requirePushTwoFloat32Consts(t *testing.T, x1, x2 float32, compiler compilerImpl) {
	err := compiler.compileConstF32(wazeroir.OperationConstF32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: x2})
	require.NoError(t, err)
}

func TestCompiler_compileBr(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Branch into nil label is interpreted as return. See BranchTarget.IsReturnTarget
		err = compiler.compileBr(wazeroir.OperationBr{Target: wazeroir.Label{Kind: wazeroir.LabelKindReturn}})
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
		forwardLabel := wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		err = compiler.compileBr(wazeroir.OperationBr{Target: forwardLabel})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with Unreachable status.
		compiler.compileExitFromNativeCode(nativeCallStatusCodeUnreachable)
		require.NoError(t, err)

		exitLabel := wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}
		err = compiler.compileBr(wazeroir.OperationBr{Target: exitLabel})
		require.NoError(t, err)

		// Emit code for the exitLabel.
		skip := compiler.compileLabel(wazeroir.OperationLabel{Label: exitLabel})
		require.False(t, skip)
		compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)
		require.NoError(t, err)

		// Emit code for the forwardLabel.
		skip = compiler.compileLabel(wazeroir.OperationLabel{Label: forwardLabel})
		require.False(t, skip)
		err = compiler.compileBr(wazeroir.OperationBr{Target: exitLabel})
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
			Types:     []wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := wazeroir.OperationCallIndirect{}

		// Place the offset value.
		err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 10})
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
			Types:     []wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := wazeroir.OperationCallIndirect{}
		targetOffset := wazeroir.OperationConstI32{Value: uint32(0)}

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
			Types:     []wasm.FunctionType{{}},
			HasTable:  true,
		})
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := wazeroir.OperationCallIndirect{}
		targetOffset := wazeroir.OperationConstI32{Value: uint32(0)}
		env.module().TypeIDs = []wasm.FunctionTypeID{1000}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]wasm.Reference, 10)
		env.addTable(&wasm.TableInstance{References: table})

		cf := &function{typeID: 50}
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
		targetType := wasm.FunctionType{
			Results:           []wasm.ValueType{wasm.ValueTypeI32},
			ResultNumInUint64: 1,
		}
		targetTypeID := wasm.FunctionTypeID(10)
		operation := wazeroir.OperationCallIndirect{TypeIndex: 0}

		table := make([]wasm.Reference, 10)
		env := newCompilerEnvironment()
		env.addTable(&wasm.TableInstance{References: table})

		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID matches the table[targetOffset]'s type ID.
		env.module().TypeIDs = make([]wasm.FunctionTypeID, 100)
		env.module().TypeIDs[operation.TypeIndex] = targetTypeID
		env.module().Engine = &moduleEngine{functions: []function{}}

		me := env.moduleEngine()
		me.functions = make([]function, len(table))
		for i := 0; i < len(table); i++ {
			// First, we create the call target function for the table element i.
			// To match its function type, it must return one value.
			expectedReturnValue := uint32(i * 1000)

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				Signature: &targetType,
			})
			err := compiler.compilePreamble()
			require.NoError(t, err)
			err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: expectedReturnValue})
			require.NoError(t, err)

			requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)
			// The function result value must be set at the bottom of the stack.
			err = compiler.compileSet(wazeroir.OperationSet{Depth: int(compiler.runtimeValueLocationStack().sp - 1)})
			require.NoError(t, err)
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			c, _, err := compiler.compile()
			require.NoError(t, err)

			// Now that we've generated the code for this function,
			// add it to the module engine and assign its pointer to the table index.
			me.functions[i] = function{
				parent:             &code{codeSegment: c},
				codeInitialAddress: uintptr(unsafe.Pointer(&c[0])),
				moduleInstance:     env.moduleInstance,
				typeID:             targetTypeID,
			}
			table[i] = uintptr(unsafe.Pointer(&me.functions[i]))
		}

		// Test to ensure that we can call all the functions stored in the table.
		for i := 1; i < len(table); i++ {
			expectedReturnValue := uint32(i * 1000)
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				compiler := env.requireNewCompiler(t, newCompiler,
					&wazeroir.CompilationResult{
						Signature: &wasm.FunctionType{},
						Types:     []wasm.FunctionType{targetType},
						HasTable:  true,
					},
				)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Place the offset value. Here we try calling a function of functionaddr == table[i].FunctionIndex.
				err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(i)})
				require.NoError(t, err)

				// At this point, we should have one item (offset value) on the stack.
				requireRuntimeLocationStackPointerEqual(t, 1, compiler)

				require.NoError(t, compiler.compileCallIndirect(operation))

				// At this point, we consumed the offset value, but the function returns one value,
				// so the stack pointer results in the same.
				requireRuntimeLocationStackPointerEqual(t, 1, compiler)

				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate the code under test and run.
				code, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, nativeCallStatusCodeReturned.String(), env.compilerStatus().String())
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, expectedReturnValue, uint32(env.ce.popValue()))
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
	operation := wazeroir.OperationCallIndirect{TypeIndex: typeIndex}
	env.module().TypeIDs = make([]wasm.FunctionTypeID, typeIndex+1)
	env.module().TypeIDs[typeIndex] = typeID
	env.module().Engine = &moduleEngine{functions: []function{}}

	types := make([]wasm.FunctionType, typeIndex+1)
	types[typeIndex] = wasm.FunctionType{}

	me := env.moduleEngine()
	{ // Compiling call target.
		compiler := env.requireNewCompiler(t, newCompiler, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		c, _, err := compiler.compile()
		require.NoError(t, err)

		f := function{
			parent:             &code{codeSegment: c},
			codeInitialAddress: uintptr(unsafe.Pointer(&c[0])),
			moduleInstance:     env.moduleInstance,
		}
		me.functions = append(me.functions, f)
		table[0] = uintptr(unsafe.Pointer(&f))
	}

	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Types:     types,
		HasTable:  true,
	})
	err := compiler.compilePreamble()
	require.NoError(t, err)

	err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 0})
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
	env := newCompilerEnvironment()
	me := env.moduleEngine()
	expectedValue := uint32(0)

	// Emit the call target function.
	const numCalls = 3
	targetFunctionType := wasm.FunctionType{
		Params:           []wasm.ValueType{wasm.ValueTypeI32},
		Results:          []wasm.ValueType{wasm.ValueTypeI32},
		ParamNumInUint64: 1, ResultNumInUint64: 1,
	}
	for i := 0; i < numCalls; i++ {
		// Each function takes one argument, adds the value with 100 + i and returns the result.
		addTargetValue := uint32(100 + i)
		expectedValue += addTargetValue
		compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
			Signature: &targetFunctionType,
		})

		err := compiler.compilePreamble()
		require.NoError(t, err)

		err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: addTargetValue})
		require.NoError(t, err)
		// Picks the function argument placed at the bottom of the stack.
		err = compiler.compilePick(wazeroir.OperationPick{Depth: int(compiler.runtimeValueLocationStack().sp - 1)})
		require.NoError(t, err)
		// Adds the const to the picked value.
		err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeI32))
		require.NoError(t, err)
		// Then store the added result into the bottom of the stack (which is treated as the result of the function).
		err = compiler.compileSet(wazeroir.OperationSet{Depth: int(compiler.runtimeValueLocationStack().sp - 1)})
		require.NoError(t, err)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		c, _, err := compiler.compile()
		require.NoError(t, err)
		me.functions = append(me.functions, function{
			parent:             &code{codeSegment: c},
			codeInitialAddress: uintptr(unsafe.Pointer(&c[0])),
			moduleInstance:     env.moduleInstance,
		})
	}

	// Now we start building the caller's code.
	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Functions: make([]uint32, numCalls),
		Types:     []wasm.FunctionType{targetFunctionType},
	})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	const initialValue = 100
	expectedValue += initialValue
	err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: 1234}) // Dummy value so the base pointer would be non-trivial for callees.
	require.NoError(t, err)
	err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: initialValue})
	require.NoError(t, err)

	// Call all the built functions.
	for i := 0; i < numCalls; i++ {
		err = compiler.compileCall(wazeroir.OperationCall{FunctionIndex: uint32(i)})
		require.NoError(t, err)
	}

	// Set the result slot
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// Check status and returned values.
	require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
	require.Equal(t, uint64(0), env.stackBasePointer())
	require.Equal(t, uint64(2), env.stackPointer()) // Must be 2 (dummy value + the calculation results)
	require.Equal(t, expectedValue, env.stackTopAsUint32())
}
