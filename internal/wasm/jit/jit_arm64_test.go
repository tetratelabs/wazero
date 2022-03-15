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
		Module: j.moduleInstance,
		Kind:   wasm.FunctionKindWasm,
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
	var ctx callEngine
	require.Equal(t, int(unsafe.Offsetof(ctx.jitCallReturnAddress)), callEngineArchContextJITCallReturnAddressOffset, "fix consts in jit_arm64.s")
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum32BitSignedInt)), callEngineArchContextMinimum32BitSignedIntOffset)
	require.Equal(t, int(unsafe.Offsetof(ctx.minimum64BitSignedInt)), callEngineArchContextMinimum64BitSignedIntOffset)
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
		err := compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)
		require.NoError(t, err)

		// Now emit the body. First we add NOP so that we can execute code after the target label.
		nop := compiler.compileNOP()

		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: backwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with unreachable status.
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

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
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Emit code for the forward label where we emit the expectedValue and then exit.
		requireAddLabel(t, compiler, forwardLabel)
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)
		require.NoError(t, err)

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
		setupFunc func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool)
	}{
		{
			name: "cond on register",
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
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
			setupFunc: func(t *testing.T, compiler *arm64Compiler, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
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
					err = compiler.compileExitFromNativeCode(unreachableStatus)
					require.NoError(t, err)

					// Emit code for .then label.
					requireAddLabel(t, compiler, thenBranchTarget.Target.Label)
					err = compiler.compileExitFromNativeCode(thenLabelExitStatus)
					require.NoError(t, err)

					// Emit code for .else label.
					requireAddLabel(t, compiler, elseBranchTarget.Target.Label)
					err = compiler.compileExitFromNativeCode(elseLabelExitStatus)
					require.NoError(t, err)

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

		err = compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)
		require.NoError(t, err)

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
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
			require.NoError(t, err)
		}

		ret := compiler.newProg()
		ret.As = obj.ARET
		ret.To.Type = obj.TYPE_REG
		ret.To.Reg = reservedRegisterForTemporary
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

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
		err = compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

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
						// If the target memory region's ceil exceeds the length of memory, we must exit the function
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
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// After the initial exec, the code must exit with builtin function call status and funcaddress for memory grow.
	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
	require.Equal(t, builtinFunctionIndexMemoryGrow, env.builtinFunctionCallAddress())

	// Reenter from the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

	// Check if the code successfully executed the code after builtin function call.
	require.Equal(t, expValue, env.stackTopAsUint32())
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestArm64Compiler_compileMemorySize(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Emit memory.size instructions.
	err = compiler.compileMemorySize()
	require.NoError(t, err)
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.locationStack.sp)
	require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().registerType())

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	require.Equal(t, uint32(defaultMemoryPageNumInTest), env.stackTopAsUint32())
}

func TestArm64Compiler_compileHostFunction(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)

	// The assembler skips the first instruction so we intentionally add NOP here.
	// TODO: delete after #233
	compiler.compileNOP()

	err := compiler.compileHostFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// On the return, the code must exit with the host call status.
	require.Equal(t, jitCallStatusCodeCallHostFunction, env.jitStatus())

	// Re-enter the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

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

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

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

func TestArm64Compiler_compileSignExtend(t *testing.T) {
	type fromKind byte
	from8, from16, from32 := fromKind(0), fromKind(1), fromKind(2)

	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int32
			expected int32
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x012345_00, expected: 0, fromKind: from8},
			{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x0123_0000, expected: 0, fromKind: from16},
			{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend32From8()
				} else {
					err = compiler.compileSignExtend32From16()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct {
			in       int64
			expected int64
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L271-L277
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x01234567_89abcd_00, expected: 0, fromKind: from8},
			{in: 81985529216486784 /* = 0xfedcba98_765432_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L279-L285
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x12345678_9abc_0000, expected: 0, fromKind: from16},
			{in: 81985529216466944 /* = 0xfedcba98_7654_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L287-L296
			{in: 0, expected: 0, fromKind: from32},
			{in: 0x7fff, expected: 32767, fromKind: from32},
			{in: 0x8000, expected: 32768, fromKind: from32},
			{in: 0xffff, expected: 65535, fromKind: from32},
			{in: 0x7fffffff, expected: 0x7fffffff, fromKind: from32},
			{in: 0x80000000, expected: -0x80000000, fromKind: from32},
			{in: 0xffffffff, expected: -1, fromKind: from32},
			{in: 0x01234567_00000000, expected: 0, fromKind: from32},
			{in: -81985529054232576 /* = 0xfedcba98_80000000 bit pattern */, expected: -0x80000000, fromKind: from32},
			{in: -1, expected: -1, fromKind: from32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend64From8()
				} else if tc.fromKind == from16 {
					err = compiler.compileSignExtend64From16()
				} else {
					err = compiler.compileSignExtend64From32()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt64())
			})
		}
	})
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
	requireRunAndExpectedValueReturned := func(t *testing.T, env *jitEnv, c *arm64Compiler, expValue uint32) {
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

			requireRunAndExpectedValueReturned(t, env, compiler, tc.expectedValue)
		})
	}
}
