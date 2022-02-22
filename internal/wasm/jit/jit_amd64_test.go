//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func (j *jitEnv) requireNewCompiler(t *testing.T) *amd64Compiler {
	b, err := asm.NewBuilder("amd64", 128)
	require.NoError(t, err)
	return &amd64Compiler{builder: b,
		locationStack: newValueLocationStack(),
		labels:        map[string]*labelInfo{},
		f:             &wasm.FunctionInstance{ModuleInstance: j.moduleInstance, FunctionKind: wasm.FunctionKindWasm},
	}
}

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

func TestAmd64Compiler_maybeGrowValueStack(t *testing.T) {
	t.Run("not grow", func(t *testing.T) {
		for _, baseOffset := range []uint64{5, 10, 20} {

			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)

			compiler.initializeReservedStackBasePointer()
			err := compiler.maybeGrowValueStack()
			require.NoError(t, err)
			require.NotNil(t, compiler.onStackPointerCeilDeterminedCallBack)

			valueStackLen := uint64(len(env.stack()))
			stackPointerCeil := uint64(5)
			stackBasePointer := valueStackLen - baseOffset // Base + Max <= valueStackLen = no need to grow!
			compiler.onStackPointerCeilDeterminedCallBack(stackPointerCeil)
			compiler.onStackPointerCeilDeterminedCallBack = nil
			env.setValueStackBasePointer(stackBasePointer)

			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run codes
			env.exec(code)

			// The status code must be "Returned", not "BuiltinFunctionCall".
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		}
	})
	t.Run("grow", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		compiler.initializeReservedStackBasePointer()
		err := compiler.maybeGrowValueStack()
		require.NoError(t, err)

		// On the return from grow value stack, we just exit with "Returned" status.
		compiler.exit(jitCallStatusCodeReturned)

		stackPointerCeil := uint64(6)
		compiler.stackPointerCeil = stackPointerCeil
		valueStackLen := uint64(len(env.stack()))
		stackBasePointer := valueStackLen - 5 // Base + Max > valueStackLen = need to grow!
		env.setValueStackBasePointer(stackBasePointer)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run codes
		env.exec(code)

		// Check if the call exits with builtin function call status.
		require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())

		// Reenter from the return address.
		returnAddress := env.callFrameStackPeek().returnAddress
		require.NotZero(t, returnAddress)
		jitcall(returnAddress, uintptr(unsafe.Pointer(env.engine())))

		// Check the result. This should be "Returned".
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestAmd64Compiler_returnFunction(t *testing.T) {
	t.Run("last return", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		const expectedValue uint32 = 100
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedValue})
		require.NoError(t, err)

		// Before returnFunction, we have the one const on the stack.
		require.Len(t, compiler.locationStack.usedRegisters, 1)
		err = compiler.returnFunction()
		require.NoError(t, err)

		// After returnFunction, all the registers must be released.
		require.Len(t, compiler.locationStack.usedRegisters, 0)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// See the previous call frame stack poitner to verify the correctness of exit decision.
		const previousCallFrameStackPointer uint64 = 50
		env.setCallFrameStackPointer(previousCallFrameStackPointer)
		env.setPreviousCallFrameStackPointer(previousCallFrameStackPointer)

		// Run codes
		env.exec(code)

		// Check the exit status and returned value.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, previousCallFrameStackPointer, env.callFrameStackPointer())
		require.Equal(t, expectedValue, env.stackTopAsUint32())
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
			err := compiler.compilePreamble()
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

func TestAmd64Compiler_initializeModuleContext(t *testing.T) {
	for _, tc := range []struct {
		name           string
		moduleInstance *wasm.ModuleInstance
	}{
		{
			name: "no nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "memory nil",
			moduleInstance: &wasm.ModuleInstance{
				Tables:  []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
				Globals: []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "memory zero length",
			moduleInstance: &wasm.ModuleInstance{
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 0)},
			},
		},
		{
			name: "table length zero",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: nil}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "table length zero part2",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 0)}},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "table nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "table nil part2",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
			},
		},
		{
			name: "globals nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:         []*wasm.TableInstance{{Table: make([]wasm.TableElement, 20)}},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.initializeReservedStackBasePointer()
			compiler.f.ModuleInstance = tc.moduleInstance

			require.Empty(t, compiler.locationStack.usedRegisters)
			err := compiler.initializeModuleContext()
			require.NoError(t, err)

			require.Empty(t, compiler.locationStack.usedRegisters)

			const expectedStatus = jitCallStatusCodeReturned
			compiler.exit(expectedStatus)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run codes
			env.exec(code)

			// Check the exit status.
			require.Equal(t, expectedStatus, env.jitStatus())

			// Check if the fields of engine.moduleContext are updated.
			engine := env.engine()

			bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Globals))
			require.Equal(t, bufSliceHeader.Data, engine.moduleContext.globalElement0Address)

			if tc.moduleInstance.MemoryInstance != nil {
				bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.MemoryInstance.Buffer))
				require.Equal(t, uint64(bufSliceHeader.Len), engine.moduleContext.memorySliceLen)
				require.Equal(t, bufSliceHeader.Data, engine.moduleContext.memoryElement0Address)
			}

			if len(tc.moduleInstance.Tables) > 0 {
				tableHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Tables[0].Table))
				require.Equal(t, uint64(tableHeader.Len), engine.moduleContext.tableSliceLen)
				require.Equal(t, tableHeader.Data, engine.moduleContext.tableElement0Address)
			}
		})
	}
}

func TestAmd64Compiler_compileBrTable(t *testing.T) {
	requireRunAndExpectedValueReturned := func(t *testing.T, c *amd64Compiler, expValue uint32) {
		// Emit code for each label which returns the frame ID.
		for returnValue := uint32(0); returnValue < 10; returnValue++ {
			label := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			c.ir.LabelCallers[label.String()] = 1
			_ = c.compileLabel(&wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(&wazeroir.OperationConstI32{Value: label.FrameID})
			err := c.releaseAllRegistersToStack()
			require.NoError(t, err)
			c.exit(jitCallStatusCodeReturned)
		}

		// Generate the code under test.
		code, _, _, err := c.compile()
		require.NoError(t, err)

		// Run codes
		env := newJITEnvironment()
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

	for _, indexReg := range unreservedGeneralPurposeIntRegisters {
		for _, tmpReg := range unreservedGeneralPurposeIntRegisters {
			if indexReg == tmpReg {
				continue
			}
			indexReg := indexReg
			tmpReg := tmpReg
			t.Run(fmt.Sprintf("index_register=%d,tmpRegister=%d", indexReg, tmpReg), func(t *testing.T) {
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

						for _, r := range unreservedGeneralPurposeIntRegisters {
							if r != indexReg && r != tmpReg {
								compiler.locationStack.markRegisterUsed(r)
							}
						}

						compiler.locationStack.pushValueLocationOnRegister(indexReg)
						compiler.movIntConstToRegister(tc.index, indexReg)

						err = compiler.compileBrTable(tc.o)
						require.NoError(t, err)

						require.NotContains(t, compiler.locationStack.usedRegisters, indexReg)
						require.NotContains(t, compiler.locationStack.usedRegisters, tmpReg)

						requireRunAndExpectedValueReturned(t, compiler, tc.expectedValue)
					})
				}
			})
		}
	}
}

func TestAmd64Compiler_pushFunctionInputs(t *testing.T) {
	f := &wasm.FunctionInstance{
		FunctionKind: wasm.FunctionKindWasm,
		FunctionType: &wasm.TypeInstance{Type: &wasm.FunctionType{Params: []wasm.ValueType{wasm.ValueTypeF64, wasm.ValueTypeI32}}}}
	compiler := &amd64Compiler{locationStack: newValueLocationStack(), f: f}
	compiler.pushFunctionParams()
	require.Equal(t, uint64(len(f.FunctionType.Type.Params)), compiler.locationStack.sp)
	loc := compiler.locationStack.pop()
	require.Equal(t, uint64(1), loc.stackPointer)
	loc = compiler.locationStack.pop()
	require.Equal(t, uint64(0), loc.stackPointer)
}

func Test_setJITStatus(t *testing.T) {
	for _, s := range []jitCallStatusCode{
		jitCallStatusCodeReturned,
		jitCallStatusCodeCallHostFunction,
		jitCallStatusCodeCallBuiltInFunction,
		jitCallStatusCodeUnreachable,
	} {
		t.Run(s.String(), func(t *testing.T) {
			env := newJITEnvironment()

			// Build codes.
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)
			compiler.exit(s)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run codes
			env.exec(code)

			// JIT status on engine must be updated.
			require.Equal(t, s, env.jitStatus())
		})
	}
}

func TestAmd64Compiler_initializeReservedRegisters(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	err := compiler.compilePreamble()
	require.NoError(t, err)
	compiler.exit(jitCallStatusCodeReturned)

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
		compiler.releaseRegisterToStack(loc)
		compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compilePick(t *testing.T) {
	o := &wazeroir.OperationPick{Depth: 1}

	// The case when the original value is already in register.
	t.Run("on reg", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set up the pick target original value.
		pickTargetLocation := compiler.locationStack.pushValueLocationOnRegister(int16(x86.REG_R10))
		pickTargetLocation.setRegisterType(generalPurposeRegisterTypeInt)
		compiler.locationStack.pushValueLocationOnStack() // Dummy value!
		compiler.movIntConstToRegister(100, pickTargetLocation.register)
		// Now insert pick code.
		err = compiler.compilePick(o)
		require.NoError(t, err)
		// Increment the picked value.
		pickedLocation := compiler.locationStack.peek()
		require.True(t, pickedLocation.onRegister())
		require.NotEqual(t, pickedLocation.register, pickTargetLocation.register)
		prog := compiler.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		compiler.addInstruction(prog)
		// To verify the behavior, we push the incremented picked value
		// to the stack.
		compiler.releaseRegisterToStack(pickedLocation)
		// Also write the original location back to the stack.
		compiler.releaseRegisterToStack(pickTargetLocation)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		actualStackPointer := env.stackPointer()
		require.Equal(t, uint64(3), actualStackPointer)
		require.Equal(t, uint64(101), env.stack()[actualStackPointer-1])
		require.Equal(t, uint64(100), env.stack()[actualStackPointer-3])
	})

	// The case when the original value is in stack.
	t.Run("on stack", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Setup the original value.
		compiler.locationStack.pushValueLocationOnStack() // Dummy value!
		pickTargetLocation := compiler.locationStack.pushValueLocationOnStack()
		env.stack()[pickTargetLocation.stackPointer] = 100
		compiler.locationStack.pushValueLocationOnStack() // Dummy value!

		// Now insert pick code.
		err = compiler.compilePick(o)
		require.NoError(t, err)

		// Increment the picked value.
		pickedLocation := compiler.locationStack.peek()
		prog := compiler.newProg()
		prog.As = x86.AINCQ
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = pickedLocation.register
		compiler.addInstruction(prog)

		err = compiler.releaseAllRegistersToStack()
		require.NoError(t, err)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		require.Equal(t, uint64(100), env.stack()[pickTargetLocation.stackPointer]) // Original value shouldn't be affected.
		require.Equal(t, uint64(4), env.stackPointer())
		require.Equal(t, uint64(101), env.stackTopAsUint64())
	})
}

func TestAmd64Compiler_compileConstI32(t *testing.T) {
	for _, v := range []uint32{1, 1 << 5, 1 << 31} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI32{Value: v}
			err = compiler.compileConstI32(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, uint64(o.Value)+1, env.stackTopAsUint64())
		})
	}
}

func TestAmd64Compiler_compileConstI64(t *testing.T) {
	for _, v := range []uint64{1, 1 << 5, 1 << 35, 1 << 63} {
		t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Now emit the const instruction.
			o := &wazeroir.OperationConstI64{Value: v}
			err = compiler.compileConstI64(o)
			require.NoError(t, err)

			// To verify the behavior, we increment and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AINCQ
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// Check the value of the top on the stack equals the const plus one.
			require.Equal(t, o.Value+1, env.stackTopAsUint64())
		})
	}
}

func TestAmd64Compiler_compileConstF32(t *testing.T) {
	for _, v := range []float32{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF32{Value: v}
			err = compiler.compileConstF32(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeFloat, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AADDSS
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, env.stackTopAsFloat32())
		})
	}
}

func TestAmd64Compiler_compileConstF64(t *testing.T) {
	for _, v := range []float64{1, -3.23, 100.123} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Now emit the const instruction.
			o := &wazeroir.OperationConstF64{Value: v}
			err = compiler.compileConstF64(o)
			require.NoError(t, err)

			// To verify the behavior, we double and push the const value
			// to the stack.
			loc := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeFloat, loc.registerType())
			prog := compiler.newProg()
			prog.As = x86.AADDSD
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = loc.register
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = loc.register
			compiler.addInstruction(prog)
			compiler.releaseRegisterToStack(loc)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// As we push the constant to the stack, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// Check the value of the top on the stack equals the squared const.
			require.Equal(t, o.Value*2, env.stackTopAsFloat64())
		})
	}
}

func TestAmd64Compiler_compileAdd(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		const x1Value uint32 = 113
		const x2Value uint32 = 41

		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, uint64(x1Value+x2Value), env.stackTopAsUint64())
	})
	t.Run("int64", func(t *testing.T) {
		const x1Value uint64 = 1 << 35
		const x2Value uint64 = 41

		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, uint64(x1Value+x2Value), env.stackTopAsUint64())
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float32
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: float32(math.Inf(1)), v2: -2.1},
			{v1: float32(math.Inf(1)), v2: 2.1},
			{v1: float32(math.Inf(-1)), v2: -2.1},
			{v1: float32(math.Inf(-1)), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.v1+tc.v2, env.stackTopAsFloat32())
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float64
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: math.Inf(1), v2: -2.1},
			{v1: math.Inf(1), v2: 2.1},
			{v1: math.Inf(-1), v2: -2.1},
			{v1: math.Inf(-1), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.v1+tc.v2, env.stackTopAsFloat64())
			})
		}
	})
}

func TestAmd64Compiler_emitEqOrNe(t *testing.T) {
	for _, instruction := range []struct {
		name string
		isEq bool
	}{
		{name: "eq", isEq: true},
		{name: "ne", isEq: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 uint32
				}{
					{x1: 100, x2: math.MaxUint32},
					{x1: math.MaxUint32, x2: math.MaxUint32},
					{x1: math.MaxUint32, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100, x2: 100},
					{x1: 200, x2: 100},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the cmp target values.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, env.stackTopAsUint64() == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, env.stackTopAsUint64() == 1)
						}
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 uint64
				}{
					{x1: 1, x2: math.MaxUint64},
					{x1: 100, x2: 200},
					{x1: 200, x2: 100},
					{x1: 1 << 56, x2: 100},
					{x1: 1 << 56, x2: 1 << 61},
					{x1: math.MaxUint64, x2: 100},
					{x1: 0, x2: 100},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the cmp target values.
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI64})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI64})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, env.stackTopAsUint64() == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, env.stackTopAsUint64() == 1)
						}
					})
				}
			})
			// For float operations, we have to reserve two int registers to deal with NaN cases.
			// So we intentionally use up the int registers with this function.
			useUpIntRegistersFunc := func(compiler *amd64Compiler) {
				for i, reg := range unreservedGeneralPurposeIntRegisters {
					compiler.locationStack.pushValueLocationOnRegister(reg)
					compiler.movIntConstToRegister(int64(i), reg)
				}
			}
			// Check the existing int values pushed by useUpIntRegistersFunc above.
			checkInitialIntValuesOnStack := func(t *testing.T, env *jitEnv) {
				stack := env.stack()
				for i := range unreservedGeneralPurposeIntRegisters {
					require.Equal(t, uint64(i), stack[i])
				}
			}
			t.Run("float32", func(t *testing.T) {
				for _, tc := range []struct {
					x1, x2 float32
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("x1=%f,x2=%f", tc.x1, tc.x2), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)
						useUpIntRegistersFunc(compiler)

						// Push the cmp target values.
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF32})
						}
						require.NoError(t, err)
						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, len(unreservedGeneralPurposeIntRegisters)+1, int(compiler.locationStack.sp))

						// To verify the behavior, we release the flag value
						// to the stack.
						err = compiler.releaseAllRegistersToStack()
						require.NoError(t, err)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, len(unreservedGeneralPurposeIntRegisters)+1, int(env.stackPointer()))
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, env.stackTopAsInt32() == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, env.stackTopAsInt32() == 1)
						}
						checkInitialIntValuesOnStack(t, env)
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
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)
						useUpIntRegistersFunc(compiler)

						// Push the cmp target values.
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions
						if instruction.isEq {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF64})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, len(unreservedGeneralPurposeIntRegisters)+1, int(compiler.locationStack.sp))

						// To verify the behavior, we push the flag value
						// to the stack.
						err = compiler.releaseAllRegistersToStack()
						require.NoError(t, err)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, len(unreservedGeneralPurposeIntRegisters)+1, int(env.stackPointer()))
						if instruction.isEq {
							require.Equal(t, tc.x1 == tc.x2, env.stackTopAsUint32() == 1)
						} else {
							require.Equal(t, tc.x1 != tc.x2, env.stackTopAsUint32() == 1)
						}
						checkInitialIntValuesOnStack(t, env)
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileEqz(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		for i, v := range []uint32{
			0, 1 << 16, math.MaxUint32,
		} {
			v := v
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Push the cmp target value.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: v})
				require.NoError(t, err)
				loc := compiler.locationStack.peek()

				// Emit the eqz instructions.
				err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt32})
				require.NoError(t, err)
				// At this point, the target value must be consumed
				// so the corresponding register must be marked unused.
				require.NotContains(t, compiler.locationStack.usedRegisters, loc.register)

				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, v == uint32(0), env.stackTopAsUint64() == 1)
			})
		}
	})
	t.Run("int64", func(t *testing.T) {
		for i, v := range []uint64{
			0, 1 << 16, 1 << 36, math.MaxUint64,
		} {
			v := v
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Push the cmp target values.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
				require.NoError(t, err)
				loc := compiler.locationStack.peek()

				// Emit the eqz instructions.
				err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt64})
				require.NoError(t, err)
				// At this point, the target value must be consumed
				// so the corresponding register must be marked unused.
				require.NotContains(t, compiler.locationStack.usedRegisters, loc.register)

				// To verify the behavior, we push the flag value
				// to the stack.
				top := compiler.locationStack.peek()
				require.True(t, top.onConditionalRegister() && !top.onRegister())
				err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
				require.NoError(t, err)
				require.True(t, !top.onConditionalRegister() && top.onRegister())
				compiler.releaseRegisterToStack(top)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, v == uint64(0), env.stackTopAsUint64() == 1)
			})
		}
	})
}

func TestAmd64Compiler_compileLe_or_Lt(t *testing.T) {
	for _, instruction := range []struct {
		name      string
		inclusive bool
	}{
		{name: "less_than_or_equal", inclusive: true},
		{name: "less_than", inclusive: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int32
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint32
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint32
					{x1: 100, x2: 200, signed: true},
					{x1: 100, x2: 100, signed: true},
					{x1: 200, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the target values.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt32
						} else {
							targetType = wazeroir.SignedTypeUint32
						}
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: targetType})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						var exp bool
						if tc.signed {
							exp = tc.x1 < tc.x2
						} else {
							exp = uint32(tc.x1) < uint32(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int64
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint64
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint64
					{x1: 100, x2: 200, signed: true},
					{x1: 200, x2: 100, signed: true},
					{x1: 1 << 56, x2: 100, signed: true},
					{x1: 1 << 56, x2: 1 << 61, signed: true},
					{x1: math.MaxInt64, x2: 100, signed: true},
					{x1: math.MinInt64, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the target values.
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt64
						} else {
							targetType = wazeroir.SignedTypeUint64
						}
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: targetType})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						var exp bool
						if tc.signed {
							exp = tc.x1 < tc.x2
						} else {
							exp = uint64(tc.x1) < uint64(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
					})
				}
			})
			t.Run("float32", func(t *testing.T) {
				for _, tc := range []struct {
					x1, x2 float32
				}{
					{x1: 100, x2: -1.1},
					{x1: -1, x2: 100},
					{x1: 100, x2: 200},
					{x1: 100.01234124, x2: 100.01234124},
					{x1: 100.01234124, x2: -100.01234124},
					{x1: 200.12315, x2: 100},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: float32(math.NaN()), x2: 0},
					{x1: 0, x2: float32(math.NaN())},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("x1=%f,x2=%f", tc.x1, tc.x2), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()

						// Prepare operands.
						require.NoError(t, err)
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat32})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat32})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						exp := tc.x1 < tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
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
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Prepare operands.
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeFloat64})
						} else {
							err = compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						exp := tc.x1 < tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileGe_or_Gt(t *testing.T) {
	for _, instruction := range []struct {
		name      string
		inclusive bool
	}{
		{name: "greater_than_or_equal", inclusive: true},
		{name: "greater_than", inclusive: false},
	} {
		instruction := instruction
		t.Run(instruction.name, func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int32
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint32
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint32
					{x1: 100, x2: 200, signed: true},
					{x1: 100, x2: 100, signed: true},
					{x1: 200, x2: 100, signed: true},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the target values.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt32
						} else {
							targetType = wazeroir.SignedTypeUint32
						}
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: targetType})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						var exp bool
						if tc.signed {
							exp = tc.x1 > tc.x2
						} else {
							exp = uint32(tc.x1) > uint32(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				for i, tc := range []struct {
					x1, x2 int64
					signed bool
				}{
					{x1: 100, x2: -1, signed: false}, // interpret x2 as max uint64
					{x1: -1, x2: -1, signed: false},  // interpret x1 and x2 as max uint32
					{x1: -1, x2: 100, signed: false}, // interpret x1 as max uint64
					{x1: 100, x2: 200, signed: true},
					{x1: 200, x2: 100, signed: true},
					{x1: 1 << 56, x2: 100, signed: true},
					{x1: 1 << 56, x2: 1 << 61, signed: true},
					{x1: math.MaxInt64, x2: 100, signed: true},
					{x1: math.MinInt64, x2: 100, signed: true},
				} {

					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Push the target values.
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x1)})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.x2)})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions.
						var targetType wazeroir.SignedType
						if tc.signed {
							targetType = wazeroir.SignedTypeInt64
						} else {
							targetType = wazeroir.SignedTypeUint64
						}
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: targetType})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: targetType})
						}
						require.NoError(t, err)

						// At this point, all the registesr must be consumed by cmp
						// so they should be marked as unused.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

						// To verify the behavior, we push the flag value
						// to the stack.
						top := compiler.locationStack.peek()
						require.True(t, top.onConditionalRegister() && !top.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(top)
						require.NoError(t, err)
						require.True(t, !top.onConditionalRegister() && top.onRegister())
						compiler.releaseRegisterToStack(top)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						var exp bool
						if tc.signed {
							exp = tc.x1 > tc.x2
						} else {
							exp = uint64(tc.x1) > uint64(tc.x2)
						}
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
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
					{x1: float32(math.NaN()), x2: 0},
					{x1: float32(math.NaN()), x2: 1.231},
					{x1: float32(math.NaN()), x2: -1.231},
					{x1: 0, x2: float32(math.NaN())},
					{x1: 1.231, x2: float32(math.NaN())},
					{x1: -1.231, x2: float32(math.NaN())},
					{x1: float32(math.Inf(1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(1)), x2: float32(math.Inf(1))},
					{x1: float32(math.Inf(-1)), x2: 100},
					{x1: 100, x2: float32(math.Inf(-1))},
					{x1: float32(math.Inf(-1)), x2: float32(math.Inf(-1))},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Prepare operands.
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat32})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeFloat32})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						exp := tc.x1 > tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
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
					{x1: math.NaN(), x2: 1.231},
					{x1: math.NaN(), x2: -1.231},
					{x1: 1.231, x2: math.NaN()},
					{x1: -1.231, x2: math.NaN()},
					{x1: math.Inf(1), x2: 100},
					{x1: 100, x2: math.Inf(1)},
					{x1: math.Inf(1), x2: math.Inf(1)},
					{x1: math.Inf(-1), x2: 100},
					{x1: 100, x2: math.Inf(-1)},
					{x1: math.Inf(-1), x2: math.Inf(-1)},
				} {
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Prepare operands.
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x1})
						require.NoError(t, err)
						x1 := compiler.locationStack.peek()
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.x2})
						require.NoError(t, err)
						x2 := compiler.locationStack.peek()

						// Emit the cmp instructions,
						if instruction.inclusive {
							err = compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeFloat64})
						} else {
							err = compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeFloat64})
						}
						require.NoError(t, err)

						// At this point, these registers must be consumed.
						require.NotContains(t, compiler.locationStack.usedRegisters, x1.register)
						require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)
						// Plus the result must be pushed.
						require.Equal(t, uint64(1), compiler.locationStack.sp)

						// To verify the behavior, we push the flag value
						// to the stack.
						flag := compiler.locationStack.peek()
						require.True(t, flag.onConditionalRegister() && !flag.onRegister())
						err = compiler.moveConditionalToFreeGeneralPurposeRegister(flag)
						require.NoError(t, err)
						require.True(t, !flag.onConditionalRegister() && flag.onRegister())
						compiler.releaseRegisterToStack(flag)
						compiler.exit(jitCallStatusCodeReturned)

						// Generate the code under test.
						// and the verification code (moving the result to the stack so we can assert against it)
						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())
						exp := tc.x1 > tc.x2
						if instruction.inclusive {
							exp = exp || tc.x1 == tc.x2
						}
						require.Equal(t, exp, env.stackTopAsUint64() == 1)
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_compileSub(t *testing.T) {
	t.Run("int32", func(t *testing.T) {
		const x1Value uint32 = 1 << 31
		const x2Value uint32 = 51
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeI32})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, uint64(x1Value-x2Value), env.stackTopAsUint64())
	})
	t.Run("int64", func(t *testing.T) {
		const x1Value uint64 = 1 << 35
		const x2Value uint64 = 51

		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x1Value})
		require.NoError(t, err)
		x1 := compiler.locationStack.peek()
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: x2Value})
		require.NoError(t, err)
		x2 := compiler.locationStack.peek()

		err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeI64})
		require.NoError(t, err)
		require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
		require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

		// To verify the behavior, we push the value
		// to the stack.
		compiler.releaseRegisterToStack(x1)
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		// Check the stack.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, x1Value-x2Value, env.stackTopAsUint64())
	})
	t.Run("float32", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float32
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: float32(math.Inf(1)), v2: -2.1},
			{v1: float32(math.Inf(1)), v2: 2.1},
			{v1: float32(math.Inf(-1)), v2: -2.1},
			{v1: float32(math.Inf(-1)), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.v1-tc.v2, env.stackTopAsFloat32())
			})
		}
	})
	t.Run("float64", func(t *testing.T) {
		for i, tc := range []struct {
			v1, v2 float64
		}{
			{v1: 1.1, v2: 2.3},
			{v1: 1.1, v2: -2.3},
			{v1: math.Inf(1), v2: -2.1},
			{v1: math.Inf(1), v2: 2.1},
			{v1: math.Inf(-1), v2: -2.1},
			{v1: math.Inf(-1), v2: 2.1},
		} {
			tc := tc
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v1})
				require.NoError(t, err)
				x1 := compiler.locationStack.peek()
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: tc.v2})
				require.NoError(t, err)
				x2 := compiler.locationStack.peek()

				err = compiler.compileSub(&wazeroir.OperationSub{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
				require.Contains(t, compiler.locationStack.usedRegisters, x1.register)
				require.NotContains(t, compiler.locationStack.usedRegisters, x2.register)

				// To verify the behavior, we push the value
				// to the stack.
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check the stack.
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.v1-tc.v2, env.stackTopAsFloat64())
			})
		}
	})
}

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
				name:  "x1:staack,x2:ax",
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

				// To verify the behavior, we push the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

				// To verify the behavior, we push the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

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
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compilClz(t *testing.T) {
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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compilCtz(t *testing.T) {
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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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
func TestAmd64Compiler_compilPopcnt(t *testing.T) {
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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.releaseAllRegistersToStack()
				require.NoError(t, err)
				compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compile_and_or_xor_shl_shr_rotl_rotr(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   wazeroir.Operation
	}{
		{name: "and-32-bit", op: &wazeroir.OperationAnd{Type: wazeroir.UnsignedInt32}},
		{name: "and-64-bit", op: &wazeroir.OperationAnd{Type: wazeroir.UnsignedInt64}},
		{name: "or-32-bit", op: &wazeroir.OperationOr{Type: wazeroir.UnsignedInt32}},
		{name: "or-64-bit", op: &wazeroir.OperationOr{Type: wazeroir.UnsignedInt64}},
		{name: "xor-32-bit", op: &wazeroir.OperationXor{Type: wazeroir.UnsignedInt32}},
		{name: "xor-64-bit", op: &wazeroir.OperationXor{Type: wazeroir.UnsignedInt64}},
		{name: "shl-32-bit", op: &wazeroir.OperationShl{Type: wazeroir.UnsignedInt32}},
		{name: "shl-64-bit", op: &wazeroir.OperationShl{Type: wazeroir.UnsignedInt64}},
		{name: "shr-signed-32-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedInt32}},
		{name: "shr-signed-64-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedInt64}},
		{name: "shr-unsigned-32-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedUint32}},
		{name: "shr-unsigned-64-bit", op: &wazeroir.OperationShr{Type: wazeroir.SignedUint64}},
		{name: "rotl-32-bit", op: &wazeroir.OperationRotl{Type: wazeroir.UnsignedInt32}},
		{name: "rotl-64-bit", op: &wazeroir.OperationRotl{Type: wazeroir.UnsignedInt64}},
		{name: "rotr-32-bit", op: &wazeroir.OperationRotr{Type: wazeroir.UnsignedInt32}},
		{name: "rotr-64-bit", op: &wazeroir.OperationRotr{Type: wazeroir.UnsignedInt64}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, locations := range []struct {
				name         string
				x1Reg, x2Reg int16
			}{
				{
					name:  "x1:cx,x2:random_reg",
					x1Reg: x86.REG_CX,
					x2Reg: x86.REG_R10,
				},
				{
					name:  "x1:cx,x2:stack",
					x1Reg: x86.REG_CX,
					x2Reg: nilRegister,
				},
				{
					name:  "x1:random_reg,x2:cx",
					x1Reg: x86.REG_R10,
					x2Reg: x86.REG_CX,
				},
				{
					name:  "x1:staack,x2:cx",
					x1Reg: nilRegister,
					x2Reg: x86.REG_CX,
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
				locations := locations
				t.Run(locations.name, func(t *testing.T) {
					for i, vs := range []struct {
						x1, x2 uint64
					}{
						{x1: 0, x2: 0},
						{x1: 0, x2: 1},
						{x1: 1, x2: 0},
						{x1: 1, x2: 1},
						{x1: 1 << 31, x2: 1},
						{x1: 1, x2: 1 << 31},
						{x1: 1 << 31, x2: 1 << 31},
						{x1: 1 << 63, x2: 1},
						{x1: 1, x2: 1 << 63},
						{x1: 1 << 63, x2: 1 << 63},
					} {
						vs := vs
						t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							var is32Bit bool
							var expectedValue uint64
							var compileOperationFunc func()
							switch o := tc.op.(type) {
							case *wazeroir.OperationAnd:
								compileOperationFunc = func() {
									err := compiler.compileAnd(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(uint32(vs.x1) & uint32(vs.x2))
								} else {
									expectedValue = vs.x1 & vs.x2
								}
							case *wazeroir.OperationOr:
								compileOperationFunc = func() {
									err := compiler.compileOr(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(uint32(vs.x1) | uint32(vs.x2))
								} else {
									expectedValue = vs.x1 | vs.x2
								}
							case *wazeroir.OperationXor:
								compileOperationFunc = func() {
									err := compiler.compileXor(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(uint32(vs.x1) ^ uint32(vs.x2))
								} else {
									expectedValue = vs.x1 ^ vs.x2
								}
							case *wazeroir.OperationShl:
								compileOperationFunc = func() {
									err := compiler.compileShl(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(uint32(vs.x1) << uint32(vs.x2%32))
								} else {
									expectedValue = vs.x1 << (vs.x2 % 64)
								}
							case *wazeroir.OperationShr:
								compileOperationFunc = func() {
									err := compiler.compileShr(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.SignedInt32 || o.Type == wazeroir.SignedUint32
								switch o.Type {
								case wazeroir.SignedInt32:
									expectedValue = uint64(int32(vs.x1) >> (uint32(vs.x2) % 32))
								case wazeroir.SignedInt64:
									expectedValue = uint64(int64(vs.x1) >> (vs.x2 % 64))
								case wazeroir.SignedUint32:
									expectedValue = uint64(uint32(vs.x1) >> (uint32(vs.x2) % 32))
								case wazeroir.SignedUint64:
									expectedValue = vs.x1 >> (vs.x2 % 64)
								}
							case *wazeroir.OperationRotl:
								compileOperationFunc = func() {
									err := compiler.compileRotl(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(bits.RotateLeft32(uint32(vs.x1), int(vs.x2)))
								} else {
									expectedValue = uint64(bits.RotateLeft64(vs.x1, int(vs.x2)))
								}
							case *wazeroir.OperationRotr:
								compileOperationFunc = func() {
									err := compiler.compileRotr(o)
									require.NoError(t, err)
								}
								is32Bit = o.Type == wazeroir.UnsignedInt32
								if is32Bit {
									expectedValue = uint64(bits.RotateLeft32(uint32(vs.x1), -int(vs.x2)))
								} else {
									expectedValue = uint64(bits.RotateLeft64(vs.x1, -int(vs.x2)))
								}
							}

							// Setup the target values.
							if locations.x1Reg != nilRegister {
								compiler.movIntConstToRegister(int64(vs.x1), locations.x1Reg)
								compiler.locationStack.pushValueLocationOnRegister(locations.x1Reg)
							} else {
								loc := compiler.locationStack.pushValueLocationOnStack()
								env.stack()[loc.stackPointer] = uint64(vs.x1)
							}
							if locations.x2Reg != nilRegister {
								compiler.movIntConstToRegister(int64(vs.x2), locations.x2Reg)
								compiler.locationStack.pushValueLocationOnRegister(locations.x2Reg)
							} else {
								loc := compiler.locationStack.pushValueLocationOnStack()
								env.stack()[loc.stackPointer] = uint64(vs.x2)
							}

							// Compile the operation.
							compileOperationFunc()
							require.Equal(t, uint64(1), compiler.locationStack.sp)

							switch tc.op.Kind() {
							case wazeroir.OperationKindShl, wazeroir.OperationKindShr,
								wazeroir.OperationKindRotl, wazeroir.OperationKindRotr:
								require.NotContains(t, compiler.locationStack.usedRegisters, x86.REG_CX)
								if locations.x1Reg == x86.REG_CX || locations.x1Reg == nilRegister {
									require.True(t, compiler.locationStack.peek().onStack())
									require.Len(t, compiler.locationStack.usedRegisters, 0)
								} else {
									require.True(t, compiler.locationStack.peek().onRegister())
									require.Len(t, compiler.locationStack.usedRegisters, 1)
								}
							}

							// To verify the behavior, we release the value
							// to the stack.
							err = compiler.releaseAllRegistersToStack()
							require.NoError(t, err)
							compiler.exit(jitCallStatusCodeReturned)

							// Generate and run the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Check the result.
							require.Equal(t, uint64(1), env.stackPointer())
							if is32Bit {
								require.Equal(t, uint32(expectedValue), env.stackTopAsUint32())
							} else {
								require.Equal(t, expectedValue, env.stackTopAsUint64())
							}
						})
					}
				})
			}
		})
	}
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
						name:  "x1:staack,x2:ax",
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

								// To verify the behavior, we push the value
								// to the stack.
								err = compiler.releaseAllRegistersToStack()
								require.NoError(t, err)
								compiler.exit(jitCallStatusCodeReturned)

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
						name:  "x1:staack,x2:ax",
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

								// We add the value previously on the DX with the quotiont of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								err = compiler.releaseAllRegistersToStack()
								require.NoError(t, err)
								compiler.exit(jitCallStatusCodeReturned)

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
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

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
				compiler.releaseRegisterToStack(x1)
				compiler.exit(jitCallStatusCodeReturned)

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
						name:  "x1:staack,x2:ax",
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

								// To verify the behavior, we push the value
								// to the stack.
								err = compiler.releaseAllRegistersToStack()
								require.NoError(t, err)
								compiler.exit(jitCallStatusCodeReturned)

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
						name:  "x1:staack,x2:ax",
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

								// We add the value previously on the DX with the quotiont of the division result
								// in order to ensure that not saving existing DX value would cause
								// the failure in a subsequent instruction.
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
								require.NoError(t, err)

								// To verify the behavior, we push the value
								// to the stack.
								err = compiler.releaseAllRegistersToStack()
								require.NoError(t, err)
								compiler.exit(jitCallStatusCodeReturned)

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

			// To verify the behavior, we release the value
			// to the stack.
			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

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

			// To verify the behavior, we release the value
			// to the stack.
			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

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
							err = compiler.releaseAllRegistersToStack()
							require.NoError(t, err)
							compiler.exit(jitCallStatusCodeReturned)

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

					// To verify the behavior, we release the value
					// to the stack.
					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

					// To verify the behavior, we release the value
					// to the stack.
					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

					// To verify the behavior, we release the value
					// to the stack.
					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

					// To verify the behavior, we release the value
					// to the stack.
					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

					// To verify the behavior, we release the value
					// to the stack.
					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_setupMemoryAccessCeil(t *testing.T) {
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
					compiler.f.ModuleInstance = env.moduleInstance

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					reg, err := compiler.setupMemoryAccessCeil(offset, targetSizeInByte)
					require.NoError(t, err)

					compiler.locationStack.pushValueLocationOnRegister(reg)

					err = compiler.releaseAllRegistersToStack()
					require.NoError(t, err)
					compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compileLoad(t *testing.T) {
	for i, tp := range []wazeroir.UnsignedType{
		wazeroir.UnsignedTypeI32,
		wazeroir.UnsignedTypeI64,
		wazeroir.UnsignedTypeF32,
		wazeroir.UnsignedTypeF64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad{Type: tp, Arg: &wazeroir.MemoryImmediate{Offset: 361}}
			err = compiler.compileLoad(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.True(t, loadedValue.onRegister())

			// Double the loaded value in order to verify the behavior.
			var addInst obj.As
			switch tp {
			case wazeroir.UnsignedTypeI32:
				require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDL
			case wazeroir.UnsignedTypeI64:
				require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
				require.True(t, isIntRegister(loadedValue.register))
				addInst = x86.AADDQ
			case wazeroir.UnsignedTypeF32:
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedValue.registerType())
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSS
			case wazeroir.UnsignedTypeF64:
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedValue.registerType())
				require.True(t, isFloatRegister(loadedValue.register))
				addInst = x86.AADDSD
			}
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = addInst
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			doubleLoadedValue.From.Type = obj.TYPE_REG
			doubleLoadedValue.From.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.exit(jitCallStatusCodeReturned)
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Place the load target value to the memory.
			targetRegion := env.memory()[baseOffset+o.Arg.Offset:]
			var expValue uint64
			switch tp {
			case wazeroir.UnsignedTypeI32:
				original := uint32(100)
				binary.LittleEndian.PutUint32(targetRegion, original)
				expValue = uint64(original * 2)
			case wazeroir.UnsignedTypeI64:
				original := uint64(math.MaxUint32 + 123) // The value exceeds 32-bit.
				binary.LittleEndian.PutUint64(targetRegion, original)
				expValue = original * 2
			case wazeroir.UnsignedTypeF32:
				original := float32(1.234)
				binary.LittleEndian.PutUint32(targetRegion, math.Float32bits(original))
				expValue = uint64(math.Float32bits(original * 2))
			case wazeroir.UnsignedTypeF64:
				original := float64(math.MaxFloat32 + 100.1) // The value exceeds 32-bit.
				binary.LittleEndian.PutUint64(targetRegion, math.Float64bits(original))
				expValue = math.Float64bits(original * 2)
			}

			// Run code.
			env.exec(code)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			require.Equal(t, expValue, env.stackTopAsUint64())
		})
	}
}

func TestAmd64Compiler_compileLoad8(t *testing.T) {
	for i, tp := range []wazeroir.SignedInt{
		wazeroir.SignedInt32,
		wazeroir.SignedInt64,
		wazeroir.SignedUint32,
		wazeroir.SignedUint64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad8{Type: tp, Arg: &wazeroir.MemoryImmediate{Offset: 361}}
			err = compiler.compileLoad8(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
			require.True(t, loadedValue.onRegister())

			// Increment the loaded value in order to verify the behavior.
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = x86.AINCB
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.exit(jitCallStatusCodeReturned)
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// For testing, arbitrary byte is be fine.
			original := byte(0x10)
			env.memory()[baseOffset+o.Arg.Offset] = byte(original)

			// Run code.
			env.exec(code)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// The loaded value must be incremented via x86.AINCB.
			require.Equal(t, original+1, env.stackTopAsByte())
		})
	}
}

func TestAmd64Compiler_compileLoad16(t *testing.T) {
	for i, tp := range []wazeroir.SignedInt{
		wazeroir.SignedInt32,
		wazeroir.SignedInt64,
		wazeroir.SignedUint32,
		wazeroir.SignedUint64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before load operations, we must push the base offset value.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[base.stackPointer] = baseOffset

			// Emit the memory load instructions.
			o := &wazeroir.OperationLoad16{Type: tp, Arg: &wazeroir.MemoryImmediate{Offset: 361}}
			err = compiler.compileLoad16(o)
			require.NoError(t, err)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			loadedValue := compiler.locationStack.peek()
			require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
			require.True(t, loadedValue.onRegister())

			// Increment the loaded value in order to verify the behavior.
			doubleLoadedValue := compiler.newProg()
			doubleLoadedValue.As = x86.AINCW
			doubleLoadedValue.To.Type = obj.TYPE_REG
			doubleLoadedValue.To.Reg = loadedValue.register
			compiler.addInstruction(doubleLoadedValue)

			// We need to write the result back to the memory stack.
			compiler.releaseRegisterToStack(loadedValue)

			// Generate the code under test.
			compiler.exit(jitCallStatusCodeReturned)
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// For testing, arbitrary uint16 is be fine.
			original := uint16(0xff_fe)
			binary.LittleEndian.PutUint16(env.memory()[baseOffset+o.Arg.Offset:], original)

			// Run code.
			env.exec(code)

			// Load instruction must push the loaded value to the top of the stack,
			// so the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
			// The loaded value must be incremented via x86.AINCW.
			require.Equal(t, original+1, env.stackTopAsUint16())
		})
	}
}

func TestAmd64Compiler_compileLoad32(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Before load operations, we must push the base offset value.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[base.stackPointer] = baseOffset

	// Emit the memory load instructions.
	o := &wazeroir.OperationLoad32{Arg: &wazeroir.MemoryImmediate{Offset: 361}}
	err = compiler.compileLoad32(o)
	require.NoError(t, err)

	// At this point, the loaded value must be on top of the stack, and placed on a register.
	loadedValue := compiler.locationStack.peek()
	require.Equal(t, generalPurposeRegisterTypeInt, loadedValue.registerType())
	require.True(t, loadedValue.onRegister())

	// Increment the loaded value in order to verify the behavior.
	doubleLoadedValue := compiler.newProg()
	doubleLoadedValue.As = x86.AINCL
	doubleLoadedValue.To.Type = obj.TYPE_REG
	doubleLoadedValue.To.Reg = loadedValue.register
	compiler.addInstruction(doubleLoadedValue)

	// We need to write the result back to the memory stack.
	compiler.releaseRegisterToStack(loadedValue)

	// Generate the code under test.
	compiler.exit(jitCallStatusCodeReturned)
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// For testing, arbitrary uint32 is be fine.
	original := uint32(0xff_ff_fe)
	binary.LittleEndian.PutUint32(env.memory()[baseOffset+o.Arg.Offset:], original)

	// Run code.
	env.exec(code)

	// Load instruction must push the loaded value to the top of the stack,
	// so the stack pointer must be incremented.
	require.Equal(t, uint64(1), env.stackPointer())
	// The loaded value must be incremented via x86.AINCL.
	require.Equal(t, original+1, env.stackTopAsUint32())
}

func TestAmd64Compiler_compileStore(t *testing.T) {
	for i, tp := range []wazeroir.UnsignedType{
		wazeroir.UnsignedTypeI32,
		wazeroir.UnsignedTypeI64,
		wazeroir.UnsignedTypeF32,
		wazeroir.UnsignedTypeF64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before store operations, we must push the base offset, and the store target values.
			const baseOffset = 100 // For testing. Arbitrary number is fine.
			base := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[base.stackPointer] = baseOffset
			storeTargetValue := uint64(math.MaxUint64)
			storeTarget := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[storeTarget.stackPointer] = storeTargetValue
			switch tp {
			case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
				storeTarget.setRegisterType(generalPurposeRegisterTypeInt)
			case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
				storeTarget.setRegisterType(generalPurposeRegisterTypeFloat)
			}

			// Emit the memory load instructions.
			o := &wazeroir.OperationStore{Type: tp, Arg: &wazeroir.MemoryImmediate{Offset: 361}}
			err = compiler.compileStore(o)
			require.NoError(t, err)

			// At this point, two values are popped so the stack pointer must be zero.
			require.Equal(t, uint64(0), compiler.locationStack.sp)
			// Plus there should be no used registers.
			require.Len(t, compiler.locationStack.usedRegisters, 0)

			// Generate the code under test.
			compiler.exit(jitCallStatusCodeReturned)
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// All the values are popped, so the stack pointer must be zero.
			require.Equal(t, uint64(0), env.stackPointer())
			// Check the stored value.
			offset := o.Arg.Offset + baseOffset
			mem := env.memory()
			switch o.Type {
			case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
				v := binary.LittleEndian.Uint32(mem[offset : offset+4])
				require.Equal(t, uint32(storeTargetValue), v)
				// The trailing bytes must be intact since this is 32-bit mov.
				v = binary.LittleEndian.Uint32(mem[offset+4 : offset+8])
				require.Equal(t, uint32(0), v)
			case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
				v := binary.LittleEndian.Uint64(mem[offset : offset+8])
				require.Equal(t, storeTargetValue, v)
			}
		})
	}
}

func TestAmd64Compiler_compileStore8(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore8{Arg: &wazeroir.MemoryImmediate{Offset: 361}}
	err = compiler.compileStore8(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.exit(jitCallStatusCodeReturned)
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), env.stackPointer())
	// Check the stored value.
	mem := env.memory()
	offset := o.Arg.Offset + baseOffset
	require.Equal(t, byte(storeTargetValue), mem[offset])
	// The trailing bytes must be intact since this is only moving one byte.
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0, 0}, mem[offset+1:offset+8])
}

func TestAmd64Compiler_compileStore16(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore16{Arg: &wazeroir.MemoryImmediate{Offset: 361}}
	err = compiler.compileStore16(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.exit(jitCallStatusCodeReturned)
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), env.stackPointer())
	// Check the stored value.
	mem := env.memory()
	offset := o.Arg.Offset + baseOffset
	require.Equal(t, uint16(storeTargetValue), binary.LittleEndian.Uint16(mem[offset:]))
	// The trailing bytes must be intact since this is only moving 2 byte.
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, mem[offset+2:offset+8])
}

func TestAmd64Compiler_compileStore32(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Before store operations, we must push the base offset, and the store target values.
	const baseOffset = 100 // For testing. Arbitrary number is fine.
	base := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[base.stackPointer] = baseOffset
	storeTargetValue := uint64(0x12_34_56_78_9a_bc_ef_01) // For testing. Arbitrary number is fine.
	storeTarget := compiler.locationStack.pushValueLocationOnStack()
	env.stack()[storeTarget.stackPointer] = storeTargetValue
	storeTarget.setRegisterType(generalPurposeRegisterTypeInt)

	// Emit the memory load instructions.
	o := &wazeroir.OperationStore32{Arg: &wazeroir.MemoryImmediate{Offset: 361}}
	err = compiler.compileStore32(o)
	require.NoError(t, err)

	// At this point, two values are popped so the stack pointer must be zero.
	require.Equal(t, uint64(0), compiler.locationStack.sp)
	// Plus there should be no used registers.
	require.Len(t, compiler.locationStack.usedRegisters, 0)

	// Generate the code under test.
	compiler.exit(jitCallStatusCodeReturned)
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// All the values are popped, so the stack pointer must be zero.
	require.Equal(t, uint64(0), env.stackPointer())
	// Check the stored value.
	mem := env.memory()
	offset := o.Arg.Offset + baseOffset
	require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
	// The trailing bytes must be intact since this is only moving 4 byte.
	require.Equal(t, []byte{0, 0, 0, 0}, mem[offset+4:offset+8])
}

func TestAmd64Compiler_compileMemoryGrow(t *testing.T) {
	for _, currentCallFrameStackPointer := range []uint64{0, 10, 20} {
		currentCallFrameStackPointer := currentCallFrameStackPointer
		t.Run(fmt.Sprintf("%d", currentCallFrameStackPointer), func(t *testing.T) {
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
			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.setCallFrameStackPointer(currentCallFrameStackPointer)
			env.exec(code)

			require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
			require.Equal(t, builtinFunctionAddressMemoryGrow, env.functionCallAddress())

			returnAddress := env.callFrameStackPeek().returnAddress
			require.NotZero(t, returnAddress)
			jitcall(returnAddress, uintptr(unsafe.Pointer(env.engine())))

			require.Equal(t, expValue, env.stackTopAsUint32())
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		})
	}
}

func TestAmd64Compiler_compileMemorySize(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t)
	compiler.f.ModuleInstance = env.moduleInstance

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Emit memory.size instructions.
	err = compiler.compileMemorySize()
	require.NoError(t, err)
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.locationStack.sp)
	require.Equal(t, generalPurposeRegisterTypeInt, compiler.locationStack.peek().registerType())

	err = compiler.releaseAllRegistersToStack()
	require.NoError(t, err)
	compiler.exit(jitCallStatusCodeReturned)

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

			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

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
	err = compiler.releaseAllRegistersToStack()
	require.NoError(t, err)
	require.Len(t, compiler.locationStack.usedRegisters, 0)
	compiler.exit(jitCallStatusCodeReturned)

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
			compiler.onStackPointerCeilDeterminedCallBack = func(acutalStackPointerCeilInCallBack uint64) {
				called = true
				require.Equal(t, expectedStackPointerCeil, acutalStackPointerCeilInCallBack)
			}

			_, _, acutalStackPointerCeil, err := compiler.compile()
			require.NoError(t, err)
			require.True(t, called)
			require.Equal(t, expectedStackPointerCeil, acutalStackPointerCeil)
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

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// Check the jitCallStatus of engine.
	require.Equal(t, jitCallStatusCodeUnreachable, env.jitStatus())
	// All the values on registers must be written back to stack.
	require.Equal(t, uint64(300), env.stack()[0])
	require.Equal(t, uint64(51), env.stack()[1])
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
				compiler.releaseRegisterToStack(x1)
			}
			compiler.exit(jitCallStatusCodeReturned)

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
			// To verify the behavior, we release all the registers to stack locations.
			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

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

func TestAmd64Compiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			// Setup the globals.
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: &wasm.GlobalType{ValType: tp}}, nil}
			env.addGlobals(globals...)
			// Compiler needs global type information at compilation time.
			compiler.f = &wasm.FunctionInstance{
				ModuleInstance: &wasm.ModuleInstance{Globals: globals},
				FunctionKind:   wasm.FunctionKindWasm,
			}

			// Emit the code.
			err := compiler.compilePreamble()
			require.NoError(t, err)
			op := &wazeroir.OperationGlobalGet{Index: 1}
			err = compiler.compileGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := compiler.locationStack.peek()
			require.True(t, global.onRegister())
			require.Len(t, compiler.locationStack.usedRegisters, 1)
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isFloatRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isIntRegister(global.register))
			}
			err = compiler.releaseAllRegistersToStack()
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run the code assembled above.
			env.exec(code)

			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, env.stack()[0])
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
		})
	}
}

func TestAmd64Compiler_compileGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t)
			compiler.f.ModuleInstance = env.moduleInstance

			// Setup the globals.
			env.addGlobals(nil, &wasm.GlobalInstance{Val: 40, Type: &wasm.GlobalType{ValType: tp}}, nil)

			// Place the set target value.
			loc := compiler.locationStack.pushValueLocationOnStack()
			env.stack()[loc.stackPointer] = valueToSet

			// Now emit the code.
			err := compiler.compilePreamble()
			require.NoError(t, err)
			op := &wazeroir.OperationGlobalSet{Index: 1}
			err = compiler.compileGlobalSet(op)
			require.NoError(t, err)
			compiler.exit(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			// The global value should be set to valueToSet.
			require.Equal(t, valueToSet, env.getGlobal(op.Index))
			// Plus we consumed the top of the stack, the stack pointer must be decremented.
			require.Equal(t, uint64(0), env.stackPointer())
		})
	}
}

func TestAmd64Compiler_callFunction(t *testing.T) {
	for _, isAddressFromRegister := range []bool{false, true} {
		isAddressFromRegister := isAddressFromRegister
		t.Run(fmt.Sprintf("is_address_from_register=%v", isAddressFromRegister), func(t *testing.T) {
			t.Run("need to grow call frame stack", func(t *testing.T) {
				env := newJITEnvironment()
				engine := env.engine()

				env.setCallFrameStackPointer(engine.globalContext.callFrameStackLen - 1)
				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				require.Empty(t, compiler.locationStack.usedRegisters)
				if isAddressFromRegister {
					err = compiler.callFunctionFromRegister(x86.REG_AX, &wasm.FunctionType{})
				} else {
					err = compiler.callFunctionFromAddress(11111 /* can be arbitrary*/, &wasm.FunctionType{})
				}
				require.NoError(t, err)
				require.Empty(t, compiler.locationStack.usedRegisters)

				// Because we must early return from the function this case,
				// we emit the undefined instruction after the callFunctionFromAddress.
				compiler.undefined()

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// If the call frame stack pointer equals the length of call frame stack length,
				// we have to call the builtin function to grow the slice.
				require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
				require.Equal(t, builtinFunctionAddressGrowCallFrameStack, env.functionCallAddress())
			})
			t.Run("stack ok", func(t *testing.T) {
				env := newJITEnvironment()
				engine := env.engine()

				// Emit the call target function.
				const numCalls = 10
				targetFunctionType := &wasm.FunctionType{
					Params:  []wasm.ValueType{wasm.ValueTypeI32},
					Results: []wasm.ValueType{wasm.ValueTypeI32},
				}

				expectedValue := uint32(0)
				moduleInstanceToExpectedValueInMemory := map[*wasm.ModuleInstance]uint32{}
				for i := 0; i < numCalls; i++ {
					// Each function takes one argument, adds the value with 100 + i and returns the result.
					addTargetValue := uint32(100 + i)
					moduleInstance := &wasm.ModuleInstance{
						MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 1024)},
					}
					moduleInstanceToExpectedValueInMemory[moduleInstance] = addTargetValue

					compiler := env.requireNewCompiler(t)
					compiler.f = &wasm.FunctionInstance{
						FunctionKind:   wasm.FunctionKindWasm,
						FunctionType:   &wasm.TypeInstance{Type: targetFunctionType},
						ModuleInstance: moduleInstance,
					}

					err := compiler.compilePreamble()
					require.NoError(t, err)

					expectedValue += addTargetValue
					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: addTargetValue})
					require.NoError(t, err)

					err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
					require.NoError(t, err)

					// Also, we modify the memory to ensure that context siwtch between module instances actually works.
					const tmpReg = x86.REG_AX
					moveValueToReg := compiler.newProg()
					moveValueToReg.As = x86.AMOVL // 32bit!
					moveValueToReg.From.Type = obj.TYPE_CONST
					moveValueToReg.From.Offset = int64(addTargetValue)
					moveValueToReg.To.Type = obj.TYPE_REG
					moveValueToReg.To.Reg = tmpReg
					compiler.addInstruction(moveValueToReg)

					writeValueToMemory := compiler.newProg()
					writeValueToMemory.As = x86.AMOVL // 32bit!
					writeValueToMemory.From.Type = obj.TYPE_REG
					writeValueToMemory.From.Reg = tmpReg
					writeValueToMemory.To.Type = obj.TYPE_MEM
					writeValueToMemory.To.Reg = reservedRegisterForMemory
					compiler.addInstruction(writeValueToMemory)

					err = compiler.returnFunction()
					require.NoError(t, err)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					compiledFunction := &compiledFunction{
						codeSegment:        code,
						codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
					}
					engine.addCompiledFunction(wasm.FunctionAddress(i), compiledFunction)
				}

				// Now we start building the caller's code.
				compiler := env.requireNewCompiler(t)
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
					if isAddressFromRegister {
						const tmpReg = x86.REG_AX
						compiler.movIntConstToRegister(int64(i), tmpReg)
						err = compiler.callFunctionFromRegister(tmpReg, targetFunctionType)
					} else {
						err = compiler.callFunctionFromAddress(wasm.FunctionAddress(i), targetFunctionType)
					}
					require.NoError(t, err)
				}

				err = compiler.returnFunction()
				require.NoError(t, err)

				// Generate the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Run code.
				env.exec(code)

				// Check status and returned values.
				require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
				require.Equal(t, uint64(2), env.stackPointer()) // Must be 2 (dummy value + the calculation results)
				require.Equal(t, uint64(0), env.stackBasePointer())
				require.Equal(t, expectedValue, env.stackTopAsUint32())

				// Also, in the middle of function call, we write the added value into each memory instance.
				for mod, expValue := range moduleInstanceToExpectedValueInMemory {
					require.Equal(t, expValue, binary.LittleEndian.Uint32(mod.MemoryInstance.Buffer[0:]))
				}
			})
		})
	}
}

func TestAmd64Compiler_compileCall(t *testing.T) {
	env := newJITEnvironment()
	engine := env.engine()

	const targetFunctionAddress wasm.FunctionAddress = 5 // arbitrary value for testing
	targetFunctionType := &wasm.FunctionType{
		Params:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32},
		Results: []wasm.ValueType{wasm.ValueTypeI32},
	}

	{
		// Call target function takes three i32 arguments and does ADD 2 times.
		compiler := env.requireNewCompiler(t)
		compiler.f = &wasm.FunctionInstance{
			ModuleInstance: &wasm.ModuleInstance{},
			FunctionKind:   wasm.FunctionKindWasm,
			FunctionType:   &wasm.TypeInstance{Type: targetFunctionType},
		}
		err := compiler.compilePreamble()
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
			require.NoError(t, err)
		}
		err = compiler.returnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		engine.addCompiledFunction(targetFunctionAddress, &compiledFunction{
			codeSegment:        code,
			codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		})
	}

	// Now we start building the caller's code.
	compiler := env.requireNewCompiler(t)
	compiler.f = &wasm.FunctionInstance{ModuleInstance: &wasm.ModuleInstance{
		Functions: []*wasm.FunctionInstance{
			{
				FunctionKind: wasm.FunctionKindWasm,
				FunctionType: &wasm.TypeInstance{Type: targetFunctionType},
				Address:      targetFunctionAddress,
			},
		},
	}}

	err := compiler.compilePreamble()
	require.NoError(t, err)

	var expectedValue uint32
	// Emit the const expressions for function arguments.
	for i := 0; i < len(targetFunctionType.Params); i++ {
		param := uint32(1 << (i + 1))
		expectedValue += param
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: param})
		require.NoError(t, err)
	}

	err = compiler.compileCall(&wazeroir.OperationCall{FunctionIndex: 0})
	require.NoError(t, err)

	err = compiler.returnFunction()
	require.NoError(t, err)

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run code.
	env.exec(code)

	// Check the status and returned value.
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	require.Equal(t, uint64(1), env.stackPointer())
	require.Equal(t, expectedValue, env.stackTopAsUint32())
}

func TestAmd64Compiler_compileCallIndirect(t *testing.T) {
	t.Run("out of bounds", func(t *testing.T) {
		env := newJITEnvironment()
		env.setTable(make([]wasm.TableElement, 10))
		compiler := env.requireNewCompiler(t)

		targetOperation := &wazeroir.OperationCallIndirect{}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex.
		compiler.f = &wasm.FunctionInstance{
			FunctionKind:   wasm.FunctionKindWasm,
			ModuleInstance: &wasm.ModuleInstance{Types: []*wasm.TypeInstance{{Type: &wasm.FunctionType{}}}},
		}

		// Place the offfset value.
		loc := compiler.locationStack.pushValueLocationOnStack()
		env.stack()[loc.stackPointer] = 10

		// Now emit the code.
		err := compiler.compilePreamble()
		require.NoError(t, err)
		require.NoError(t, compiler.compileCallIndirect(targetOperation))
		compiler.exit(jitCallStatusCodeReturned)

		// Generate the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("uninitialized", func(t *testing.T) {
		env := newJITEnvironment()
		table := make([]wasm.TableElement, 10)
		env.setTable(table)

		compiler := env.requireNewCompiler(t)
		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		compiler.f = &wasm.FunctionInstance{
			ModuleInstance: &wasm.ModuleInstance{Types: []*wasm.TypeInstance{{Type: &wasm.FunctionType{}, TypeID: 1000}}},
			FunctionKind:   wasm.FunctionKindWasm,
		}
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table[0] = wasm.TableElement{FunctionTypeID: wasm.UninitializedTableElementTypeID}

		// Place the offfset value.
		err := compiler.compileConstI32(targetOffset)
		require.NoError(t, err)

		// Now emit the code.
		err = compiler.compilePreamble()
		require.NoError(t, err)
		require.NoError(t, compiler.compileCallIndirect(targetOperation))

		// Generate the code under test.
		compiler.exit(jitCallStatusCodeReturned)
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("type not match", func(t *testing.T) {
		env := newJITEnvironment()
		table := make([]wasm.TableElement, 10)
		env.setTable(table)

		compiler := env.requireNewCompiler(t)
		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		env.moduleInstance.Types = []*wasm.TypeInstance{{Type: &wasm.FunctionType{}, TypeID: 1000}}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table[0] = wasm.TableElement{FunctionTypeID: 50}

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Place the offfset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)

		// Now emit the code.
		require.NoError(t, compiler.compileCallIndirect(targetOperation))

		// Generate the code under test.
		compiler.exit(jitCallStatusCodeReturned)
		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, jitCallStatusCodeTypeMismatchOnIndirectCall, env.jitStatus())
	})

	t.Run("ok", func(t *testing.T) {
		targetType := &wasm.FunctionType{
			Params:  []wasm.ValueType{},
			Results: []wasm.ValueType{wasm.ValueTypeI32}}
		targetTypeID := wasm.FunctionTypeID(10) // Arbitrary number is fine for testing.
		operation := &wazeroir.OperationCallIndirect{TypeIndex: 0}
		moduleInstance := &wasm.ModuleInstance{Types: make([]*wasm.TypeInstance, 100)}
		moduleInstance.Types[operation.TableIndex] = &wasm.TypeInstance{Type: targetType, TypeID: targetTypeID}

		table := make([]wasm.TableElement, 10)
		for i := 0; i < len(table); i++ {
			table[i] = wasm.TableElement{FunctionAddress: wasm.FunctionAddress(i), FunctionTypeID: targetTypeID}
		}

		for i := 0; i < len(table); i++ {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				env := newJITEnvironment()
				env.setTable(table)
				engine := env.engine()

				// First we creat the call target function with function address = i,
				// and it returns one value.
				expectedReturnValue := uint32(i * 1000)
				{
					compiler := env.requireNewCompiler(t)
					err := compiler.compilePreamble()
					require.NoError(t, err)
					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
					require.NoError(t, err)
					err = compiler.returnFunction()
					require.NoError(t, err)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					engine.addCompiledFunction(table[i].FunctionAddress, &compiledFunction{
						codeSegment:        code,
						codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
					})
				}

				compiler := env.requireNewCompiler(t)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Ensure that the module instance has the type information for targetOperation.TypeIndex,
				compiler.f = &wasm.FunctionInstance{ModuleInstance: moduleInstance, FunctionKind: wasm.FunctionKindWasm}
				// and the typeID  matches the table[targetOffset]'s type ID.

				// Place the offfset value. Here we try calling a function of functionaddr == table[i].FunctionAddress.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(i)})
				require.NoError(t, err)

				// At this point, we should have one item (offset value) on the stack.
				require.Equal(t, uint64(1), compiler.locationStack.sp)

				// Now emit the code.

				require.NoError(t, compiler.compileCallIndirect(operation))

				// At this point, we consumed the offset value, but the function returns one value,
				// so the stack pointer results in the same.
				require.Equal(t, uint64(1), compiler.locationStack.sp)

				err = compiler.returnFunction()
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
	})
}

func TestAmd64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.readInstructionAddress(x86.REG_AX, obj.AJMP)

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
		compiler.readInstructionAddress(destinationRegister, obj.ARET)

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

		err = compiler.returnFunction()
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
