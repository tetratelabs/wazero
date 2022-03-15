//go:build a

package jit

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

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
