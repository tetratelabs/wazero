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
