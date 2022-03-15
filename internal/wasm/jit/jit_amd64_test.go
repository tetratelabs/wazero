//go:build a

package jit

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func (c *amd64Compiler) movIntConstToRegister(val int64, targetRegister int16) *obj.Prog {
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	c.addInstruction(prog)
	return prog
}

func TestAmd64Compiler_compileBrTable(t *testing.T) {
	requireRunAndExpectedValueReturned := func(t *testing.T, env *jitEnv, c *amd64Compiler, expValue uint32) {
		// Emit code for each label which returns the frame ID.
		for returnValue := uint32(0); returnValue < 10; returnValue++ {
			label := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			c.ir.LabelCallers[label.String()] = 1
			_ = c.compileLabel(&wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(&wazeroir.OperationConstI32{Value: label.FrameID})
			require.NoError(t, c.compileReturnFunction())
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
			t.Parallel()
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

func TestAmd64Compiler_pushFunctionInputs(t *testing.T) {
	f := &wasm.FunctionInstance{
		Kind: wasm.FunctionKindWasm,
		Type: &wasm.FunctionType{Params: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32}}}
	compiler := &amd64Compiler{locationStack: newValueLocationStack(), f: f}
	compiler.pushFunctionParams()
	require.Equal(t, uint64(len(f.Type.Params)), compiler.locationStack.sp)
	loc := compiler.locationStack.pop()
	require.Equal(t, uint64(1), loc.stackPointer)
	loc = compiler.locationStack.pop()
	require.Equal(t, uint64(0), loc.stackPointer)
}

func TestAmd64Compiler_initializeReservedRegisters(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)
	compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	newJITEnvironment().exec(code)
}

func TestAmd64Compiler_allocateRegister(t *testing.T) {
	t.Run("free", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		reg, err := compiler.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		reg, err = compiler.allocateRegister(generalPurposeRegisterTypeFloat)
		require.NoError(t, err)
		require.True(t, isFloatRegister(reg))
	})
	t.Run("steal", func(t *testing.T) {
		const stealTarget = x86.REG_AX
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		// Use up all the Int regs.
		for _, r := range unreservedGeneralPurposeIntRegisters {
			if r != stealTarget {
				compiler.locationStack.markRegisterUsed(r)
			}
		}
		stealTargetLocation := compiler.locationStack.pushValueLocationOnRegister(stealTarget)
		compiler.movIntConstToRegister(int64(50), stealTargetLocation.register)
		require.Equal(t, int16(stealTarget), stealTargetLocation.register)
		require.True(t, stealTargetLocation.onRegister())
		reg, err := compiler.allocateRegister(generalPurposeRegisterTypeInt)
		require.NoError(t, err)
		require.True(t, isIntRegister(reg))
		require.False(t, stealTargetLocation.onRegister())

		// Create new value using the stolen register.
		loc := compiler.locationStack.pushValueLocationOnRegister(reg)
		compiler.movIntConstToRegister(int64(2000), loc.register)
		compiler.compileReleaseRegisterToStack(loc)
		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the sp and value.
		require.Equal(t, uint64(2), env.stackPointer())
		require.Equal(t, []uint64{50, 2000}, env.stack()[:env.stackPointer()])
	})
}

func TestAmd64Compiler_compileLabel(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	labelKey := label.String()
	var called bool
	compiler.labels[labelKey] = &labelInfo{
		labelBeginningCallbacks: []func(*obj.Prog){func(p *obj.Prog) { called = true }},
		initialStack:            newValueLocationStack(),
	}

	// If callers > 0, the label must not be skipped.
	skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
	require.False(t, skip)
	require.NotNil(t, compiler.labels[labelKey].initialInstruction)
	require.True(t, called)

	// Otherwise, skip.
	compiler.labels[labelKey].initialStack = nil
	skip = compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
	require.True(t, skip)
}

// TODO: comment why this is arch-dependent.
func TestAmd64Compiler_compileMul(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			// Interpret -1 as stack.
			x1Reg, x2Reg int16
		}{
			{
				name:  "x1:ax,x2:random_reg",
				x1Reg: x86.REG_AX,
				x2Reg: x86.REG_R10,
			},
			{
				name:  "x1:ax,x2:stack",
				x1Reg: x86.REG_AX,
				x2Reg: -1,
			},
			{
				name:  "x1:random_reg,x2:ax",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:stack,x2:ax",
				x1Reg: -1,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:random_reg,x2:random_reg",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:stack,x2:random_reg",
				x1Reg: -1,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:random_reg,x2:stack",
				x1Reg: x86.REG_R9,
				x2Reg: -1,
			},
			{
				name:  "x1:stack,x2:stack",
				x1Reg: -1,
				x2Reg: -1,
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				env := newJITEnvironment()

				const x1Value uint32 = 1 << 11
				const x2Value uint32 = 51
				const dxValue uint64 = 111111

				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
				// Here, we put it just before two operands as ["any value used by DX", x1, x2]
				// but in reality, it can exist in any position of stack.
				compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
				prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

				// Setup values.
				if tc.x1Reg != nilRegister {
					compiler.movIntConstToRegister(int64(x1Value), tc.x1Reg)
					compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
				} else {
					loc := compiler.locationStack.pushValueLocationOnStack()
					env.stack()[loc.stackPointer] = uint64(x1Value)
				}
				if tc.x2Reg != nilRegister {
					compiler.movIntConstToRegister(int64(x2Value), tc.x2Reg)
					compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
				} else {
					loc := compiler.locationStack.pushValueLocationOnStack()
					env.stack()[loc.stackPointer] = uint64(x2Value)
				}

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
				require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
				require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
				require.Equal(t, uint64(2), compiler.locationStack.sp)
				require.Len(t, compiler.locationStack.usedRegisters, 1)
				// At this point, the previous value on the DX register is saved to the stack.
				require.True(t, prevOnDX.onStack())

				// We add the value previously on the DX with the multiplication result
				// in order to ensure that not saving existing DX value would cause
				// the failure in a subsequent instruction.
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				// Run code.
				env.exec(code)

				// Verify the stack is in the form of ["any value previously used by DX" + x1 * x2]
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, uint64(x1Value*x2Value)+dxValue, env.stackTopAsUint64())
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			x1Reg, x2Reg int16
		}{
			{
				name:  "x1:ax,x2:random_reg",
				x1Reg: x86.REG_AX,
				x2Reg: x86.REG_R10,
			},
			{
				name:  "x1:ax,x2:stack",
				x1Reg: x86.REG_AX,
				x2Reg: nilRegister,
			},
			{
				name:  "x1:random_reg,x2:ax",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:stack,x2:ax",
				x1Reg: nilRegister,
				x2Reg: x86.REG_AX,
			},
			{
				name:  "x1:random_reg,x2:random_reg",
				x1Reg: x86.REG_R10,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:stack,x2:random_reg",
				x1Reg: nilRegister,
				x2Reg: x86.REG_R9,
			},
			{
				name:  "x1:random_reg,x2:stack",
				x1Reg: x86.REG_R9,
				x2Reg: nilRegister,
			},
			{
				name:  "x1:stack,x2:stack",
				x1Reg: nilRegister,
				x2Reg: nilRegister,
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				const x1Value uint64 = 1 << 35
				const x2Value uint64 = 51
				const dxValue uint64 = 111111

				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
				// Here, we put it just before two operands as ["any value used by DX", x1, x2]
				// but in reality, it can exist in any position of stack.
				compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
				prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

				// Setup values.
				if tc.x1Reg != nilRegister {
					compiler.movIntConstToRegister(int64(x1Value), tc.x1Reg)
					compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
				} else {
					loc := compiler.locationStack.pushValueLocationOnStack()
					env.stack()[loc.stackPointer] = uint64(x1Value)
				}
				if tc.x2Reg != nilRegister {
					compiler.movIntConstToRegister(int64(x2Value), tc.x2Reg)
					compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
				} else {
					loc := compiler.locationStack.pushValueLocationOnStack()
					env.stack()[loc.stackPointer] = uint64(x2Value)
				}

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
				require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
				require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
				require.Equal(t, uint64(2), compiler.locationStack.sp)
				require.Len(t, compiler.locationStack.usedRegisters, 1)
				// At this point, the previous value on the DX register is saved to the stack.
				require.True(t, prevOnDX.onStack())

				// We add the value previously on the DX with the multiplication result
				// in order to ensure that not saving existing DX value would cause
				// the failure in a subsequent instruction.
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Verify the stack is in the form of ["any value previously used by DX" + x1 * x2]
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, uint64(x1Value*x2Value)+dxValue, env.stackTopAsUint64())
			})
		}
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float32
		}{
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: float32(math.Inf(1)), x2: 100},
			{x1: float32(math.Inf(-1)), x2: 100},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.compileReleaseRegisterToStack(x1)
				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.x1*tc.x2, env.stackTopAsFloat32())
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float64
		}{
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
			{x1: math.Inf(1), x2: 100},
			{x1: math.Inf(-1), x2: 100},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileMul(&wazeroir.OperationMul{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.compileReleaseRegisterToStack(x1)
				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.x1*tc.x2, env.stackTopAsFloat64())
			})
		}
	})
}
func TestAmd64Compiler_compileClz(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedLeadingZeros uint32 }{
			{input: 0xff_ff_ff_ff, expectedLeadingZeros: 0},
			{input: 0xf0_00_00_00, expectedLeadingZeros: 0},
			{input: 0x00_ff_ff_ff, expectedLeadingZeros: 8},
			{input: 0, expectedLeadingZeros: 32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)
				// Setup the target value.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileClz(&wazeroir.OperationClz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileClz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expectedLeadingZeros, env.stackTopAsUint32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedLeadingZeros uint64 }{
			{input: 0xf0_00_00_00_00_00_00_00, expectedLeadingZeros: 0},
			{input: 0xff_ff_ff_ff_ff_ff_ff_ff, expectedLeadingZeros: 0},
			{input: 0x00_ff_ff_ff_ff_ff_ff_ff, expectedLeadingZeros: 8},
			{input: 0x00_00_00_00_ff_ff_ff_ff, expectedLeadingZeros: 32},
			{input: 0x00_00_00_00_00_ff_ff_ff, expectedLeadingZeros: 40},
			{input: 0, expectedLeadingZeros: 64},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.expectedLeadingZeros), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the target value.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileClz(&wazeroir.OperationClz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileClz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expectedLeadingZeros, env.stackTopAsUint64())
			})
		}
	})
}

func TestAmd64Compiler_compileCtz(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedTrailingZeros uint32 }{
			{input: 0xff_ff_ff_ff, expectedTrailingZeros: 0},
			{input: 0x00_00_00_01, expectedTrailingZeros: 0},
			{input: 0xff_ff_ff_00, expectedTrailingZeros: 8},
			{input: 0, expectedTrailingZeros: 32},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the target value.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileCtz(&wazeroir.OperationCtz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileCtz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expectedTrailingZeros, env.stackTopAsUint32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedTrailingZeros uint64 }{
			{input: 0xff_ff_ff_ff_ff_ff_ff_ff, expectedTrailingZeros: 0},
			{input: 0x00_00_00_00_00_00_00_01, expectedTrailingZeros: 0},
			{input: 0xff_ff_ff_ff_ff_ff_ff_00, expectedTrailingZeros: 8},
			{input: 0xff_ff_ff_ff_00_00_00_00, expectedTrailingZeros: 32},
			{input: 0xff_ff_ff_00_00_00_00_00, expectedTrailingZeros: 40},
			{input: 0, expectedTrailingZeros: 64},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.input), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)
				// Setup the target value.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compileCtz(&wazeroir.OperationCtz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())
				// On darwin, we have two branches and one must jump to the next
				// instruction after compileCtz.
				require.True(t, runtime.GOOS != "darwin" || len(compiler.setJmpOrigins) > 0)

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expectedTrailingZeros, env.stackTopAsUint64())
			})
		}
	})
}
func TestAmd64Compiler_compilePopcnt(t *testing.T) {
	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct{ input, expectedSetBits uint32 }{
			{input: 0xff_ff_ff_ff, expectedSetBits: 32},
			{input: 0x00_00_00_01, expectedSetBits: 1},
			{input: 0x10_00_00_00, expectedSetBits: 1},
			{input: 0x00_00_10_00, expectedSetBits: 1},
			{input: 0x00_01_00_01, expectedSetBits: 2},
			{input: 0xff_ff_00_ff, expectedSetBits: 24},
			{input: 0, expectedSetBits: 0},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%032b", tc.input), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)
				// Setup the target value.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.input})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expectedSetBits, env.stackTopAsUint32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct{ in, exp uint64 }{
			{in: 0xff_ff_ff_ff_ff_ff_ff_ff, exp: 64},
			{in: 0x00_00_00_00_00_00_00_01, exp: 1},
			{in: 0x00_00_00_01_00_00_00_00, exp: 1},
			{in: 0x10_00_00_00_00_00_00_00, exp: 1},
			{in: 0xf0_00_00_00_00_00_01_00, exp: 5},
			{in: 0xff_ff_ff_ff_ff_ff_ff_00, exp: 56},
			{in: 0xff_ff_ff_00_ff_ff_ff_ff, exp: 56},
			{in: 0xff_ff_ff_ff_00_00_00_00, exp: 32},
			{in: 0xff_ff_ff_00_00_00_00_00, exp: 24},
			{in: 0, exp: 0},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%064b", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the target value.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: tc.in})
				require.NoError(t, err)

				// Emit the clz instruction.
				err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// Verify that the result is pushed, meaning that
				// stack pointer must not be changed.
				require.Equal(t, uint64(1), compiler.locationStack.sp)
				// Also the location must be register.
				require.True(t, compiler.locationStack.peek().onRegister())

				require.NoError(t, compiler.compileReturnFunction())

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.exp, env.stackTopAsUint64())
			})
		}
	})
}

func TestAmd64Compiler_compileDiv(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{{name: "signed", signed: true}, {name: "unsigned", signed: false}} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name         string
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: nilRegister,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: nilRegister,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: nilRegister,
						x2Reg: nilRegister,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for _, vs := range []struct {
							x1Value, x2Value uint32
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 2},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							{x1Value: 0x80000000, x2Value: 0xffffffff}, // This is equivalent to (-2^31 / -1) and results in overflow.
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 1},
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 0xfffffffe /* -2 in signed 32bit */},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d/%d", vs.x1Value, vs.x2Value), func(t *testing.T) {

								env := newJITEnvironment()
								compiler := env.requireNewCompiler(t)
								err := compiler.compilePreamble()
								require.NoError(t, err)

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x2Value)
								}

								if signed.signed {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeInt32})
								} else {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeUint32})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the multiplication result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
								require.NoError(t, err)

								require.NoError(t, compiler.compileReturnFunction())

								// Generate the code under test.
								code, _, _, err := compiler.compile()
								require.NoError(t, err)

								// Run code.
								env.exec(code)

								if vs.x2Value == 0 {
									require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									return
								} else if signed.signed && int32(vs.x2Value) == -1 && int32(vs.x1Value) == int32(math.MinInt32) {
									// (-2^31 / -1) = 2 ^31 is larger than the upper limit of 32-bit signed integer.
									require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									return
								}

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), env.stackPointer())
								if signed.signed {
									require.Equal(t, int32(vs.x1Value)/int32(vs.x2Value)+int32(dxValue), env.stackTopAsInt32())
								} else {
									require.Equal(t, vs.x1Value/vs.x2Value+uint32(dxValue), env.stackTopAsUint32())
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			{name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name         string
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: nilRegister,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: nilRegister,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: nilRegister,
						x2Reg: nilRegister,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for _, vs := range []struct {
							x1Value, x2Value uint64
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 1},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							{x1Value: 0x8000000000000000, x2Value: 0xffffffffffffffff}, // This is equivalent to (-2^63 / -1) and results in overflow.
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 1},
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 0xfffffffffffffffe /* -2 in signed 64bit */},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d/%d", vs.x1Value, vs.x2Value), func(t *testing.T) {

								env := newJITEnvironment()
								compiler := env.requireNewCompiler(t)
								err := compiler.compilePreamble()
								require.NoError(t, err)

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x2Value)
								}

								if signed.signed {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeInt64})
								} else {
									err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeUint64})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_AX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the quotient of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								require.NoError(t, compiler.compileReturnFunction())

								// Generate the code under test.
								code, _, _, err := compiler.compile()
								require.NoError(t, err)

								// Run code.
								env.exec(code)

								if vs.x2Value == 0 {
									require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									return
								} else if signed.signed && int64(vs.x2Value) == -1 && int64(vs.x1Value) == int64(math.MinInt64) {
									// (-2^63 / -1) = 2 ^63 is larger than the upper limit of 64-bit signed integer.
									require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									return
								}

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), env.stackPointer())
								if signed.signed {
									require.Equal(t, int64(vs.x1Value)/int64(vs.x2Value)+int64(dxValue), env.stackTopAsInt64())
								} else {
									require.Equal(t, vs.x1Value/vs.x2Value+dxValue, env.stackTopAsUint64())
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float32
		}{
			{x1: 100, x2: 0},
			{x1: 0, x2: 100},
			{x1: 100, x2: -1.1},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: float32(math.Inf(1)), x2: 100},
			{x1: float32(math.Inf(-1)), x2: -100},
			{x1: 100, x2: float32(math.Inf(1))},
			{x1: -100, x2: float32(math.Inf(-1))},
			{x1: float32(math.Inf(1)), x2: 0},
			{x1: float32(math.Inf(-1)), x2: 0},
			{x1: 0, x2: float32(math.Inf(1))},
			{x1: 0, x2: float32(math.Inf(-1))},
			{x1: float32(math.NaN()), x2: 0},
			{x1: 0, x2: float32(math.NaN())},
			{x1: float32(math.NaN()), x2: 12321},
			{x1: 12313, x2: float32(math.NaN())},
			{x1: float32(math.NaN()), x2: float32(math.NaN())},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeFloat32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.compileReleaseRegisterToStack(x1)
				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the result.
				require.Equal(t, uint64(1), env.stackPointer())
				exp := tc.x1 / tc.x2
				actual := env.stackTopAsFloat32()
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, tc.x1/tc.x2, actual)
				}
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			x1, x2 float64
		}{
			{x1: 100, x2: -1.1},
			{x1: 100, x2: 0},
			{x1: 0, x2: 0},
			{x1: -1, x2: 100},
			{x1: 100, x2: 200},
			{x1: 100.01234124, x2: 100.01234124},
			{x1: 100.01234124, x2: -100.01234124},
			{x1: 200.12315, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
			{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
			{x1: math.Inf(1), x2: 100},
			{x1: math.Inf(1), x2: -100},
			{x1: 100, x2: math.Inf(1)},
			{x1: -100, x2: math.Inf(1)},
			{x1: math.Inf(-1), x2: 100},
			{x1: math.Inf(-1), x2: -100},
			{x1: 100, x2: math.Inf(-1)},
			{x1: -100, x2: math.Inf(-1)},
			{x1: math.Inf(1), x2: 0},
			{x1: math.Inf(-1), x2: 0},
			{x1: 0, x2: math.Inf(1)},
			{x1: 0, x2: math.Inf(-1)},
			{x1: math.NaN(), x2: 0},
			{x1: 0, x2: math.NaN()},
			{x1: math.NaN(), x2: 12321},
			{x1: 12313, x2: math.NaN()},
			{x1: math.NaN(), x2: math.NaN()},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileDiv(&wazeroir.OperationDiv{Type: wazeroir.SignedTypeFloat64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.compileReleaseRegisterToStack(x1)
				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the result.
				require.Equal(t, uint64(1), env.stackPointer())
				exp := tc.x1 / tc.x2
				actual := env.stackTopAsFloat64()
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, tc.x1/tc.x2, actual)
				}
			})
		}
	})
}

func TestAmd64Compiler_compileRem(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			{name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name         string
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: nilRegister,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: nilRegister,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: nilRegister,
						x2Reg: nilRegister,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for _, vs := range []struct {
							x1Value, x2Value uint32
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 2},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 1},
							{x1Value: 0xffffffff /* -1 in signed 32bit */, x2Value: 0xfffffffe /* -2 in signed 32bit */},
							{x1Value: math.MaxInt32, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt32 + 1, x2Value: math.MaxUint32},
						} {
							vs := vs
							t.Run(fmt.Sprintf("x1=%d,x2=%d", vs.x1Value, vs.x2Value), func(t *testing.T) {

								env := newJITEnvironment()
								compiler := env.requireNewCompiler(t)
								err := compiler.compilePreamble()
								require.NoError(t, err)

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x2Value)
								}

								if signed.signed {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt32})
								} else {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint32})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_DX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the remainder result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
								require.NoError(t, err)

								require.NoError(t, compiler.compileReturnFunction())

								// Generate the code under test.
								code, _, _, err := compiler.compile()
								require.NoError(t, err)

								// Run code.
								env.exec(code)
								if vs.x2Value == 0 {
									require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									return
								}

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), env.stackPointer())
								if signed.signed {
									x1Signed := int32(vs.x1Value)
									x2Signed := int32(vs.x2Value)
									require.Equal(t, x1Signed%x2Signed+int32(dxValue), env.stackTopAsInt32())
								} else {
									require.Equal(t, vs.x1Value%vs.x2Value+uint32(dxValue), env.stackTopAsUint32())
								}
							})
						}
					})
				}
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for _, signed := range []struct {
			name   string
			signed bool
		}{
			{name: "signed", signed: true},
			{name: "unsigned", signed: false},
		} {
			signed := signed
			t.Run(signed.name, func(t *testing.T) {
				for _, tc := range []struct {
					name         string
					x1Reg, x2Reg int16
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: x86.REG_AX,
						x2Reg: x86.REG_R10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: x86.REG_AX,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: nilRegister,
						x2Reg: x86.REG_AX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: x86.REG_R10,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: nilRegister,
						x2Reg: x86.REG_R9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: x86.REG_R9,
						x2Reg: nilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: nilRegister,
						x2Reg: nilRegister,
					},
				} {
					tc := tc
					t.Run(tc.name, func(t *testing.T) {
						const dxValue uint64 = 111111
						for i, vs := range []struct {
							x1Value, x2Value uint64
						}{
							{x1Value: 2, x2Value: 1},
							{x1Value: 1, x2Value: 2},
							{x1Value: 0, x2Value: 1},
							{x1Value: 1, x2Value: 0},
							{x1Value: 0, x2Value: 0},
							// Following cases produce different resulting bit patterns for signed and unsigned.
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 1},
							{x1Value: 0xffffffffffffffff /* -1 in signed 64bit */, x2Value: 0xfffffffffffffffe /* -2 in signed 64bit */},
							{x1Value: math.MaxInt32, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt32 + 1, x2Value: math.MaxUint32},
							{x1Value: math.MaxInt64, x2Value: math.MaxUint64},
							{x1Value: math.MaxInt64 + 1, x2Value: math.MaxUint64},
						} {
							vs := vs
							t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
								env := newJITEnvironment()
								compiler := env.requireNewCompiler(t)
								err := compiler.compilePreamble()
								require.NoError(t, err)

								// Pretend there was an existing value on the DX register. We expect compileDivForInts to save this to the stack.
								// Here, we put it just before two operands as ["any value used by DX", x1, x2]
								// but in reality, it can exist in any position of stack.
								compiler.movIntConstToRegister(int64(dxValue), x86.REG_DX)
								prevOnDX := compiler.locationStack.pushValueLocationOnRegister(x86.REG_DX)

								// Setup values.
								if tc.x1Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x1Value), tc.x1Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x1Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x1Value)
								}
								if tc.x2Reg != nilRegister {
									compiler.movIntConstToRegister(int64(vs.x2Value), tc.x2Reg)
									compiler.locationStack.pushValueLocationOnRegister(tc.x2Reg)
								} else {
									loc := compiler.locationStack.pushValueLocationOnStack()
									env.stack()[loc.stackPointer] = uint64(vs.x2Value)
								}

								if signed.signed {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt64})
								} else {
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint64})
								}
								require.NoError(t, err)

								require.Equal(t, int16(x86.REG_DX), compiler.locationStack.peek().register)
								require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().regType)
								require.Equal(t, uint64(2), compiler.locationStack.sp)
								require.Len(t, compiler.locationStack.usedRegisters, 1)
								// At this point, the previous value on the DX register is saved to the stack.
								require.True(t, prevOnDX.onStack())

								// We add the value previously on the DX with the quotient of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								require.NoError(t, compiler.compileReturnFunction())

								// Generate the code under test.
								code, _, _, err := compiler.compile()
								require.NoError(t, err)

								// Run code.
								env.exec(code)
								if vs.x2Value == 0 {
									require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									return
								}

								// Verify the stack is in the form of ["any value previously used by DX" + x1 / x2]
								require.Equal(t, uint64(1), env.stackPointer())
								if signed.signed {
									require.Equal(t, int64(vs.x1Value)%int64(vs.x2Value)+int64(dxValue), env.stackTopAsInt64())
								} else {
									require.Equal(t, vs.x1Value%vs.x2Value+dxValue, env.stackTopAsUint64())
								}
							})
						}
					})
				}
			})
		}
	})
}

func TestAmd64Compiler_compileF32DemoteFromF64(t *testing.T) {
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

			require.NoError(t, compiler.compileReturnFunction())

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
				require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
			} else {
				exp := float32(v)
				actual := env.stackTopAsFloat32()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestAmd64Compiler_compileF64PromoteFromF32(t *testing.T) {
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

			require.NoError(t, compiler.compileReturnFunction())

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(float64(v)) { // NaN cannot be compared with themselves, so we have to use IsNaN
				require.True(t, math.IsNaN(env.stackTopAsFloat64()))
			} else {
				exp := float64(v)
				actual := env.stackTopAsFloat64()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestAmd64Compiler_compileReinterpret(t *testing.T) {
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

							// To verify the behavior, we release the value
							// to the stack.
							require.NoError(t, compiler.compileReturnFunction())

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

func TestAmd64Compiler_compileExtend(t *testing.T) {
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

					require.NoError(t, compiler.compileReturnFunction())

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

func TestAmd64Compiler_compileSignExtend(t *testing.T) {
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

				require.NoError(t, compiler.compileReturnFunction())

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

				require.NoError(t, compiler.compileReturnFunction())

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

func TestAmd64Compiler_compileITruncFromF(t *testing.T) {
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
				0, 100, -100, 1, -1,
				100.01234124, -100.01234124, 200.12315,
				6.8719476736e+10, /* = 1 << 36 */
				-6.8719476736e+10,
				1.37438953472e+11, /* = 1 << 37 */
				-1.37438953472e+11,
				-2147483649.0,
				2147483648.0,
				math.MinInt32,
				math.MaxInt32,
				math.MaxUint32,
				math.MinInt64,
				math.MaxInt64,
				math.MaxUint64,
				math.MaxFloat32,
				math.SmallestNonzeroFloat32,
				math.MaxFloat64,
				math.SmallestNonzeroFloat64,
				math.Inf(1), math.Inf(-1), math.NaN(),
			} {
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

				t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
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

					require.NoError(t, compiler.compileReturnFunction())

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					expStatus := jitCallStatusCodeReturned
					if math.IsNaN(v) { // NaN cannot be compared with themselves, so we have to use IsNaN
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
					} else {
						t.Fatal()
					}

					require.Equal(t, expStatus, env.jitStatus())
				})
			}
		})
	}
}

func TestAmd64Compiler_compileFConvertFromI(t *testing.T) {
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
				math.MaxUint32, math.MaxUint64, math.MaxInt32, math.MaxInt64,
				math.Float64bits(1.23455555),
				math.Float64bits(-1.23455555),
				math.Float64bits(math.NaN()),
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
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

					require.NoError(t, compiler.compileReturnFunction())

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
					} else {
						t.Fatal()
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compile_abs_neg_ceil_floor(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "abs-32-bit", op: &wazeroir.OperationAbs{Type: wazeroir.Float32}},
		{name: "abs-64-bit", op: &wazeroir.OperationAbs{Type: wazeroir.Float64}},
		{name: "neg-32-bit", op: &wazeroir.OperationNeg{Type: wazeroir.Float32}},
		{name: "neg-64-bit", op: &wazeroir.OperationNeg{Type: wazeroir.Float64}},
		{name: "ceil-32-bit", op: &wazeroir.OperationCeil{Type: wazeroir.Float32}},
		{name: "ceil-64-bit", op: &wazeroir.OperationCeil{Type: wazeroir.Float64}},
		{name: "floor-32-bit", op: &wazeroir.OperationFloor{Type: wazeroir.Float32}},
		{name: "floor-64-bit", op: &wazeroir.OperationFloor{Type: wazeroir.Float64}},
		{name: "trunc-32-bit", op: &wazeroir.OperationTrunc{Type: wazeroir.Float32}},
		{name: "trunc-64-bit", op: &wazeroir.OperationTrunc{Type: wazeroir.Float64}},
		{name: "sqrt-32-bit", op: &wazeroir.OperationSqrt{Type: wazeroir.Float32}},
		{name: "sqrt-64-bit", op: &wazeroir.OperationSqrt{Type: wazeroir.Float64}},
		{name: "nearest-32-bit", op: &wazeroir.OperationNearest{Type: wazeroir.Float32}},
		{name: "nearest-64-bit", op: &wazeroir.OperationNearest{Type: wazeroir.Float64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for i, v := range []uint64{
				0,
				1 << 63,
				1<<63 | 12345,
				1 << 31,
				1<<31 | 123455,
				6.8719476736e+10,
				math.Float64bits(-4.5), // This produces the different result between math.Round and ROUND with 0x00 mode.
				1.37438953472e+11,
				math.Float64bits(-1.3),
				uint64(math.Float32bits(-1231.123)),
				math.Float64bits(1.3),
				math.Float64bits(100.3),
				math.Float64bits(-100.3),
				uint64(math.Float32bits(1231.123)),
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
				math.Float64bits(math.NaN()),
			} {
				t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					var is32Bit bool
					var expFloat32 float32
					var expFloat64 float64
					var compileOperationFunc func()
					switch o := tc.op.(type) {
					case *wazeroir.OperationAbs:
						compileOperationFunc = func() {
							err := compiler.compileAbs(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Abs(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Abs(math.Float64frombits(v))
						}
					case *wazeroir.OperationNeg:
						compileOperationFunc = func() {
							err := compiler.compileNeg(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = -math.Float32frombits(uint32(v))
						} else {
							expFloat64 = -math.Float64frombits(v)
						}
					case *wazeroir.OperationCeil:
						compileOperationFunc = func() {
							err := compiler.compileCeil(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Ceil(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Ceil(math.Float64frombits(v))
						}
					case *wazeroir.OperationFloor:
						compileOperationFunc = func() {
							err := compiler.compileFloor(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Floor(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Floor(math.Float64frombits(v))
						}
					case *wazeroir.OperationTrunc:
						compileOperationFunc = func() {
							err := compiler.compileTrunc(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Trunc(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Trunc(math.Float64frombits(v))
						}
					case *wazeroir.OperationSqrt:
						compileOperationFunc = func() {
							err := compiler.compileSqrt(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Sqrt(float64(math.Float32frombits(uint32(v)))))
						} else {
							expFloat64 = math.Sqrt(math.Float64frombits(v))
						}
					case *wazeroir.OperationNearest:
						compileOperationFunc = func() {
							err := compiler.compileNearest(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = moremath.WasmCompatNearestF32(math.Float32frombits(uint32(v)))
						} else {
							expFloat64 = moremath.WasmCompatNearestF64(math.Float64frombits(v))
						}
					}

					// Setup the target values.
					if is32Bit {
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
					} else {
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
					}
					require.NoError(t, err)

					// Compile the operation.
					compileOperationFunc()

					require.NoError(t, compiler.compileReturnFunction())

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					require.Equal(t, uint64(1), env.stackPointer())
					if is32Bit {
						actual := env.stackTopAsFloat32()
						// NaN cannot be compared with themselves, so we have to use IsNaN
						if math.IsNaN(float64(expFloat32)) {
							require.True(t, math.IsNaN(float64(actual)))
						} else {
							require.Equal(t, actual, expFloat32)
						}
					} else {
						actual := env.stackTopAsFloat64()
						if math.IsNaN(expFloat64) { // NaN cannot be compared with themselves, so we have to use IsNaN
							require.True(t, math.IsNaN(actual))
						} else {
							require.Equal(t, expFloat64, actual)
						}
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compile_min_max_copysign(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "min-32-bit", op: &wazeroir.OperationMin{Type: wazeroir.Float32}},
		{name: "min-64-bit", op: &wazeroir.OperationMin{Type: wazeroir.Float64}},
		{name: "max-32-bit", op: &wazeroir.OperationMax{Type: wazeroir.Float32}},
		{name: "max-64-bit", op: &wazeroir.OperationMax{Type: wazeroir.Float64}},
		{name: "copysign-32-bit", op: &wazeroir.OperationCopysign{Type: wazeroir.Float32}},
		{name: "copysign-64-bit", op: &wazeroir.OperationCopysign{Type: wazeroir.Float64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, vs := range []struct{ x1, x2 float64 }{
				{x1: 100, x2: -1.1},
				{x1: 100, x2: 0},
				{x1: 0, x2: 0},
				{x1: 1, x2: 1},
				{x1: -1, x2: 100},
				{x1: 100, x2: 200},
				{x1: 100.01234124, x2: 100.01234124},
				{x1: 100.01234124, x2: -100.01234124},
				{x1: 200.12315, x2: 100},
				{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 100},
				{x1: 6.8719476736e+10 /* = 1 << 36 */, x2: 1.37438953472e+11 /* = 1 << 37*/},
				{x1: math.Inf(1), x2: 100},
				{x1: math.Inf(1), x2: -100},
				{x1: 100, x2: math.Inf(1)},
				{x1: -100, x2: math.Inf(1)},
				{x1: math.Inf(-1), x2: 100},
				{x1: math.Inf(-1), x2: -100},
				{x1: 100, x2: math.Inf(-1)},
				{x1: -100, x2: math.Inf(-1)},
				{x1: math.Inf(1), x2: 0},
				{x1: math.Inf(-1), x2: 0},
				{x1: 0, x2: math.Inf(1)},
				{x1: 0, x2: math.Inf(-1)},
				{x1: math.NaN(), x2: 0},
				{x1: 0, x2: math.NaN()},
				{x1: math.NaN(), x2: 12321},
				{x1: 12313, x2: math.NaN()},
				{x1: math.NaN(), x2: math.NaN()},
			} {
				t.Run(fmt.Sprintf("x1=%f_x2=%f", vs.x1, vs.x2), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					var is32Bit bool
					var expFloat32 float32
					var expFloat64 float64
					var compileOperationFunc func()
					switch o := tc.op.(type) {
					case *wazeroir.OperationMin:
						compileOperationFunc = func() {
							err := compiler.compileMin(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(moremath.WasmCompatMin(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = moremath.WasmCompatMin(vs.x1, vs.x2)
						}
					case *wazeroir.OperationMax:
						compileOperationFunc = func() {
							err := compiler.compileMax(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(moremath.WasmCompatMax(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = moremath.WasmCompatMax(vs.x1, vs.x2)
						}
					case *wazeroir.OperationCopysign:
						compileOperationFunc = func() {
							err := compiler.compileCopysign(o)
							require.NoError(t, err)
						}
						is32Bit = o.Type == wazeroir.Float32
						if is32Bit {
							expFloat32 = float32(math.Copysign(float64(float32(vs.x1)), float64(float32(vs.x2))))
						} else {
							expFloat64 = math.Copysign(vs.x1, vs.x2)
						}
					}

					// Setup the target values.
					if is32Bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(vs.x1)})
						require.NoError(t, err)
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(vs.x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: vs.x1})
						require.NoError(t, err)
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: vs.x2})
						require.NoError(t, err)
					}

					// Compile the operation.
					compileOperationFunc()

					require.NoError(t, compiler.compileReturnFunction())

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					require.Equal(t, uint64(1), env.stackPointer())
					if is32Bit {
						actual := env.stackTopAsFloat32()
						// NaN cannot be compared with themselves, so we have to use IsNaN
						if math.IsNaN(float64(expFloat32)) {
							require.True(t, math.IsNaN(float64(actual)), actual)
						} else {
							require.Equal(t, expFloat32, actual)
						}
					} else {
						actual := env.stackTopAsFloat64()
						if math.IsNaN(expFloat64) { // NaN cannot be compared with themselves, so we have to use IsNaN
							require.True(t, math.IsNaN(actual), actual)
						} else {
							require.Equal(t, expFloat64, actual)
						}
					}
				})
			}
		})
	}
}

func TestAmd64Compiler_compileMemoryAccessCeilSetup(t *testing.T) {
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
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t)

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					reg, err := compiler.compileMemoryAccessCeilSetup(offset, targetSizeInByte)
					require.NoError(t, err)

					compiler.locationStack.pushValueLocationOnRegister(reg)

					require.NoError(t, compiler.compileReturnFunction())

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
						require.Equal(t, uint64(ceil), env.stackTopAsUint64())
					}
				})
			}
		}
	}
}

func TestAmd64Compiler_compileMemoryGrow(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)

	err := compiler.compilePreamble()
	require.NoError(t, err)
	// Emit memory.grow instructions.
	err = compiler.compileMemoryGrow()
	require.NoError(t, err)

	// Emit arbitrary code after memory.grow returned.
	const expValue uint32 = 100
	err = compiler.compilePreamble()
	require.NoError(t, err)
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
	require.NoError(t, err)
	require.NoError(t, compiler.compileReturnFunction())

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
	require.Equal(t, builtinFunctionIndexMemoryGrow, env.builtinFunctionCallAddress())

	returnAddress := env.callFrameStackPeek().returnAddress
	require.NotZero(t, returnAddress)
	jitcall(returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

	require.Equal(t, expValue, env.stackTopAsUint32())
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestAmd64Compiler_compileMemorySize(t *testing.T) {
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

	require.NoError(t, compiler.compileReturnFunction())

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	require.Equal(t, uint32(defaultMemoryPageNumInTest), env.stackTopAsUint32())
}

func TestAmd64Compiler_compileDrop(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compileDrop(&wazeroir.OperationDrop{})
		require.NoError(t, err)
	})
	t.Run("zero start", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		shouldPeek := compiler.locationStack.pushValueLocationOnStack()
		const numReg = 10
		for i := int16(0); i < numReg; i++ {
			compiler.locationStack.pushValueLocationOnRegister(i)
		}
		err := compiler.compileDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: 0, End: numReg - 1},
		})
		require.NoError(t, err)
		for i := int16(0); i < numReg; i++ {
			require.NotContains(t, compiler.locationStack.usedRegisters, i)
		}
		actualPeek := compiler.locationStack.peek()
		require.Equal(t, shouldPeek, actualPeek)
	})
	t.Run("live all on register", func(t *testing.T) {
		const (
			numLive = 3
			dropNum = 5
		)
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		shouldBottom := compiler.locationStack.pushValueLocationOnStack()
		for i := int16(0); i < dropNum; i++ {
			compiler.locationStack.pushValueLocationOnRegister(i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			compiler.locationStack.pushValueLocationOnRegister(i)
		}
		err := compiler.compileDrop(&wazeroir.OperationDrop{
			Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
		})
		require.NoError(t, err)
		for i := int16(0); i < dropNum; i++ {
			require.NotContains(t, compiler.locationStack.usedRegisters, i)
		}
		for i := int16(dropNum); i < numLive+dropNum; i++ {
			require.Contains(t, compiler.locationStack.usedRegisters, i)
		}
		for i := int16(0); i < numLive; i++ {
			actual := compiler.locationStack.pop()
			require.True(t, actual.onRegister())
			require.Equal(t, numLive+dropNum-1-i, actual.register)
		}
		require.Equal(t, uint64(1), compiler.locationStack.sp)
		require.Equal(t, shouldBottom, compiler.locationStack.pop())
	})
	t.Run("live on stack", func(t *testing.T) {
		// This is for testing all the edge cases with fake registers.
		t.Run("fake registers", func(t *testing.T) {
			const (
				numLive        = 3
				dropNum        = 5
				liveRegisterID = 10
			)
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			bottom := compiler.locationStack.pushValueLocationOnStack()
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			for i := int16(0); i < dropNum; i++ {
				compiler.locationStack.pushValueLocationOnRegister(i)
			}
			// The bottom live value is on the stack.
			bottomLive := compiler.locationStack.pushValueLocationOnStack()
			// The second live value is on the register.
			LiveRegister := compiler.locationStack.pushValueLocationOnRegister(liveRegisterID)
			// The top live value is on the conditional.
			topLive := compiler.locationStack.pushValueLocationOnConditionalRegister(conditionalRegisterStateAE)
			require.True(t, topLive.onConditionalRegister())
			err := compiler.compileDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: numLive, End: numLive + dropNum - 1},
			})
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			require.NoError(t, err)
			require.Equal(t, uint64(4), compiler.locationStack.sp)
			for i := int16(0); i < dropNum; i++ {
				require.NotContains(t, compiler.locationStack.usedRegisters, i)
			}
			// Top value should be on the register.
			actualTopLive := compiler.locationStack.pop()
			require.True(t, actualTopLive.onRegister() && !actualTopLive.onConditionalRegister())
			require.Equal(t, topLive, actualTopLive)
			// Second one should be on the same register.
			actualLiveRegister := compiler.locationStack.pop()
			require.Equal(t, LiveRegister, actualLiveRegister)
			// The bottom live value should be moved onto the stack.
			actualBottomLive := compiler.locationStack.pop()
			require.Equal(t, bottomLive, actualBottomLive)
			require.True(t, actualBottomLive.onRegister() && !actualBottomLive.onStack())
			// The bottom after drop should stay on stack.
			actualBottom := compiler.locationStack.pop()
			require.Equal(t, uint64(0), compiler.locationStack.stack[0].stackPointer)
			require.Equal(t, bottom, actualBottom)
			require.True(t, bottom.onStack())
		})
		t.Run("real", func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			bottom := compiler.locationStack.pushValueLocationOnRegister(x86.REG_R10)
			compiler.locationStack.pushValueLocationOnRegister(x86.REG_R9)
			top := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[top.stackPointer] = 5000
			compiler.movIntConstToRegister(300, bottom.register)

			err = compiler.compileDrop(&wazeroir.OperationDrop{
				Range: &wazeroir.InclusiveRange{Start: 1, End: 1},
			})
			require.NoError(t, err)

			require.NoError(t, compiler.compileReturnFunction())

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// Check the stack.
			require.Equal(t, uint64(2), env.stackPointer())
			require.Equal(t, []uint64{
				300,
				5000, // top value should be moved to the dropped position.
			}, env.stack()[:env.stackPointer()])
		})
	})
}

func TestAmd64Compiler_releaseAllRegistersToStack(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	_ = compiler.locationStack.pushValueLocationOnStack()
	env.stack()[0] = 100
	compiler.locationStack.pushValueLocationOnRegister(x1Reg)
	compiler.locationStack.pushValueLocationOnRegister(x2Reg)
	_ = compiler.locationStack.pushValueLocationOnStack()
	env.stack()[3] = 123
	require.Len(t, compiler.locationStack.usedRegisters, 2)

	// Set the values supposed to be released to stack memory space.
	compiler.movIntConstToRegister(300, x1Reg)
	compiler.movIntConstToRegister(51, x2Reg)
	compiler.compileReleaseAllRegistersToStack()
	require.Len(t, compiler.locationStack.usedRegisters, 0)
	compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// Check the stack.
	require.Equal(t, uint64(4), env.stackPointer())
	sp := env.stackPointer()
	stack := env.stack()
	require.Equal(t, uint64(123), stack[sp-1])
	require.Equal(t, uint64(51), stack[sp-2])
	require.Equal(t, uint64(300), stack[sp-3])
	require.Equal(t, uint64(100), stack[sp-4])
}

func TestAmd64Compiler_generate(t *testing.T) {
	t.Run("max pointer", func(t *testing.T) {
		getCompiler := func(t *testing.T) (compiler *amd64Compiler) {
			env := newJITEnvironment()
			compiler = env.requireNewCompiler(t)
			ret := compiler.newProg()
			ret.As = obj.ARET
			compiler.addInstruction(ret)
			return
		}
		verify := func(t *testing.T, compiler *amd64Compiler, expectedStackPointerCeil uint64) {
			var called bool
			compiler.onStackPointerCeilDeterminedCallBack = func(actualStackPointerCeilInCallBack uint64) {
				called = true
				require.Equal(t, expectedStackPointerCeil, actualStackPointerCeilInCallBack)
			}

			_, _, actualStackPointerCeil, err := compiler.compile()
			require.NoError(t, err)
			require.True(t, called)
			require.Equal(t, expectedStackPointerCeil, actualStackPointerCeil)
		}
		t.Run("current one win", func(t *testing.T) {
			compiler := getCompiler(t)
			const expectedStackPointerCeil uint64 = 100
			compiler.stackPointerCeil = expectedStackPointerCeil
			compiler.locationStack.stackPointerCeil = expectedStackPointerCeil - 1
			verify(t, compiler, expectedStackPointerCeil)
		})
		t.Run("previous one win", func(t *testing.T) {
			compiler := getCompiler(t)
			const expectedStackPointerCeil uint64 = 100
			compiler.locationStack.stackPointerCeil = expectedStackPointerCeil
			compiler.stackPointerCeil = expectedStackPointerCeil - 1
			verify(t, compiler, expectedStackPointerCeil)
		})
	})

	t.Run("on generate callback", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		ret := compiler.newProg()
		ret.As = obj.ARET
		compiler.addInstruction(ret)

		var codePassedInCallBack []byte
		compiler.onGenerateCallbacks = append(compiler.onGenerateCallbacks, func(code []byte) error {
			codePassedInCallBack = code
			return nil
		})
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		require.NotEmpty(t, code)
		require.Equal(t, code, codePassedInCallBack)
	})
}

func TestAmd64Compiler_compileUnreachable(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	x1Reg := int16(x86.REG_AX)
	x2Reg := int16(x86.REG_R10)
	compiler.locationStack.pushValueLocationOnRegister(x1Reg)
	compiler.locationStack.pushValueLocationOnRegister(x2Reg)
	compiler.movIntConstToRegister(300, x1Reg)
	compiler.movIntConstToRegister(51, x2Reg)
	err = compiler.compileUnreachable()
	require.NoError(t, err)

	// Generate the code under test and run.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// Check the jit status.
	require.Equal(t, jitCallStatusCodeUnreachable, env.jitStatus())
}

func TestAmd64Compiler_compileSelect(t *testing.T) {
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
	const x1Value, x2Value = 100, 200
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
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			var x1, x2, c *valueLocation
			if tc.x1OnRegister {
				x1 = compiler.locationStack.pushValueLocationOnRegister(x86.REG_AX)
				compiler.movIntConstToRegister(x1Value, x1.register)
			} else {
				x1 = compiler.locationStack.pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = x1Value
			}
			if tc.x2OnRegister {
				x2 = compiler.locationStack.pushValueLocationOnRegister(x86.REG_R10)
				compiler.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 = compiler.locationStack.pushValueLocationOnStack()
				env.stack()[x2.stackPointer] = x2Value
			}
			if tc.condlValueOnStack {
				c = compiler.locationStack.pushValueLocationOnStack()
				if tc.selectX1 {
					env.stack()[c.stackPointer] = 1
				} else {
					env.stack()[c.stackPointer] = 0
				}
			} else if tc.condValueOnGPRegister {
				c = compiler.locationStack.pushValueLocationOnRegister(x86.REG_R9)
				if tc.selectX1 {
					compiler.movIntConstToRegister(1, c.register)
				} else {
					compiler.movIntConstToRegister(0, c.register)
				}
			} else if tc.condValueOnCondRegister {
				compiler.movIntConstToRegister(0, x86.REG_CX)
				cmp := compiler.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_CX
				cmp.To.Type = obj.TYPE_CONST
				if tc.selectX1 {
					cmp.To.Offset = 0
				} else {
					cmp.To.Offset = 1
				}
				compiler.addInstruction(cmp)
				compiler.locationStack.pushValueLocationOnConditionalRegister(conditionalRegisterStateE)
			}

			// Now emit code for select.
			err = compiler.compileSelect()
			require.NoError(t, err)
			// The code generation should not affect the x1's placement in any case.
			require.Equal(t, tc.x1OnRegister, x1.onRegister())
			// Plus x1 is top of the stack.
			require.Equal(t, x1, compiler.locationStack.peek())

			// Now write back the x1 to the memory if it is on a register.
			if tc.x1OnRegister {
				compiler.compileReleaseRegisterToStack(x1)
			}
			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

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
}

func TestAmd64Compiler_compileSwap(t *testing.T) {
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
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			if tc.x2OnRegister {
				x2 := compiler.locationStack.pushValueLocationOnRegister(x86.REG_R10)
				compiler.movIntConstToRegister(x2Value, x2.register)
			} else {
				x2 := compiler.locationStack.pushValueLocationOnStack()
				env.stack()[x2.stackPointer] = uint64(x2Value)
			}
			_ = compiler.locationStack.pushValueLocationOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := compiler.locationStack.pushValueLocationOnRegister(x86.REG_AX)
				compiler.movIntConstToRegister(x1Value, x1.register)
			} else if !tc.x1OnConditionalRegister {
				x1 := compiler.locationStack.pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = uint64(x1Value)
			} else {
				compiler.movIntConstToRegister(0, x86.REG_AX)
				cmp := compiler.newProg()
				cmp.As = x86.ACMPQ
				cmp.From.Type = obj.TYPE_REG
				cmp.From.Reg = x86.REG_AX
				cmp.To.Type = obj.TYPE_CONST
				cmp.To.Offset = 0
				cmp.To.Offset = 0
				compiler.addInstruction(cmp)
				compiler.locationStack.pushValueLocationOnConditionalRegister(conditionalRegisterStateE)
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

func TestAmd64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.compileReadInstructionAddress(x86.REG_AX, obj.AJMP)

		// If generate the code without JMP after readInstructionAddress,
		// the call back added must return error.
		_, _, _, err = compiler.compile()
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		const destinationRegister = x86.REG_AX
		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		compiler.compileReadInstructionAddress(destinationRegister, obj.ARET)

		// Jump to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		jmpToAfterRet := compiler.newProg()
		jmpToAfterRet.As = obj.AJMP
		jmpToAfterRet.To.Type = obj.TYPE_REG
		jmpToAfterRet.To.Reg = destinationRegister
		compiler.addInstruction(jmpToAfterRet)

		ret := compiler.newProg()
		ret.As = obj.ARET
		compiler.addInstruction(ret)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the jmp instruction above
		// must target here.
		const expectedReturnValue uint32 = 10000
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
		require.NoError(t, err)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expectedReturnValue, env.stackTopAsUint32())
	})
}
