package compiler

import (
	"encoding/hex"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestAmd64Compiler_indirectCallWithTargetOnCallingConvReg is the regression test for #526.
// In short, the offset register for call_indirect might be the same as amd64CallingConventionDestinationFunctionModuleInstanceAddressRegister
// and that must not be a failure.
func TestAmd64Compiler_indirectCallWithTargetOnCallingConvReg(t *testing.T) {
	env := newCompilerEnvironment()
	table := make([]wasm.Reference, 1)
	env.addTable(&wasm.TableInstance{References: table})
	// Ensure that the module instance has the type information for targetOperation.TypeIndex,
	// and the typeID  matches the table[targetOffset]'s type ID.
	operation := wazeroir.OperationCallIndirect{TypeIndex: 0}
	env.module().TypeIDs = []wasm.FunctionTypeID{0}
	env.module().Engine = &moduleEngine{functions: []function{}}

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
			typeID:             0,
		}
		me.functions = append(me.functions, f)
		table[0] = uintptr(unsafe.Pointer(&f))
	}

	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Types:     []wasm.FunctionType{{}},
		HasTable:  true,
	}).(*amd64Compiler)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Place the offset into the calling-convention reserved register.
	offsetLoc := compiler.pushRuntimeValueLocationOnRegister(amd64CallingConventionDestinationFunctionModuleInstanceAddressRegister,
		runtimeValueTypeI32)
	compiler.assembler.CompileConstToRegister(amd64.MOVQ, 0, offsetLoc.register)

	require.NoError(t, compiler.compileCallIndirect(operation))

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test and run.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)
}

func TestAmd64Compiler_compile_Mul_Div_Rem(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindMul,
		wazeroir.OperationKindDiv,
		wazeroir.OperationKindRem,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			t.Run("int32", func(t *testing.T) {
				tests := []struct {
					name         string
					x1Reg, x2Reg asm.Register
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: amd64.RegAX,
						x2Reg: amd64.RegR10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: amd64.RegAX,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: amd64.RegR9,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: asm.NilRegister,
						x2Reg: asm.NilRegister,
					},
				}

				for _, tt := range tests {
					tc := tt
					t.Run(tc.name, func(t *testing.T) {
						env := newCompilerEnvironment()

						const x1Value uint32 = 1 << 11
						const x2Value uint32 = 51
						const dxValue uint64 = 111111

						compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(dxValue), amd64.RegDX)
						prevOnDX := compiler.pushRuntimeValueLocationOnRegister(amd64.RegDX, runtimeValueTypeI32)

						// Setup values.
						if tc.x1Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x1Value), tc.x1Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x1Reg, runtimeValueTypeI32)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							loc.valueType = runtimeValueTypeI32
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x2Value), tc.x2Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x2Reg, runtimeValueTypeI32)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							loc.valueType = runtimeValueTypeI32
							env.stack()[loc.stackPointer] = uint64(x2Value)
						}

						switch kind {
						case wazeroir.OperationKindDiv:
							err = compiler.compileDiv(wazeroir.NewOperationDiv(wazeroir.SignedTypeUint32))
						case wazeroir.OperationKindMul:
							err = compiler.compileMul(wazeroir.NewOperationMul(wazeroir.UnsignedTypeI32))
						case wazeroir.OperationKindRem:
							err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedUint32))
						}
						require.NoError(t, err)

						require.Equal(t, registerTypeGeneralPurpose, compiler.runtimeValueLocationStack().peek().getRegisterType())
						requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)
						require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeI32))
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, err := compiler.compile()
						require.NoError(t, err)
						// Run code.
						env.exec(code)

						// Verify the stack is in the form of ["any value previously used by DX" + the result of operation]
						require.Equal(t, uint64(1), env.stackPointer())
						switch kind {
						case wazeroir.OperationKindDiv:
							require.Equal(t, x1Value/x2Value+uint32(dxValue), env.stackTopAsUint32())
						case wazeroir.OperationKindMul:
							require.Equal(t, x1Value*x2Value+uint32(dxValue), env.stackTopAsUint32())
						case wazeroir.OperationKindRem:
							require.Equal(t, x1Value%x2Value+uint32(dxValue), env.stackTopAsUint32())
						}
					})
				}
			})
			t.Run("int64", func(t *testing.T) {
				tests := []struct {
					name         string
					x1Reg, x2Reg asm.Register
				}{
					{
						name:  "x1:ax,x2:random_reg",
						x1Reg: amd64.RegAX,
						x2Reg: amd64.RegR10,
					},
					{
						name:  "x1:ax,x2:stack",
						x1Reg: amd64.RegAX,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:random_reg,x2:ax",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:stack,x2:ax",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegAX,
					},
					{
						name:  "x1:random_reg,x2:random_reg",
						x1Reg: amd64.RegR10,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:stack,x2:random_reg",
						x1Reg: asm.NilRegister,
						x2Reg: amd64.RegR9,
					},
					{
						name:  "x1:random_reg,x2:stack",
						x1Reg: amd64.RegR9,
						x2Reg: asm.NilRegister,
					},
					{
						name:  "x1:stack,x2:stack",
						x1Reg: asm.NilRegister,
						x2Reg: asm.NilRegister,
					},
				}

				for _, tt := range tests {
					tc := tt
					t.Run(tc.name, func(t *testing.T) {
						const x1Value uint64 = 1 << 35
						const x2Value uint64 = 51
						const dxValue uint64 = 111111

						env := newCompilerEnvironment()
						compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Pretend there was an existing value on the DX register. We expect compileMul to save this to the stack.
						// Here, we put it just before two operands as ["any value used by DX", x1, x2]
						// but in reality, it can exist in any position of stack.
						compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(dxValue), amd64.RegDX)
						prevOnDX := compiler.pushRuntimeValueLocationOnRegister(amd64.RegDX, runtimeValueTypeI64)

						// Setup values.
						if tc.x1Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x1Value), tc.x1Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x1Reg, runtimeValueTypeI64)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							loc.valueType = runtimeValueTypeI64
							env.stack()[loc.stackPointer] = uint64(x1Value)
						}
						if tc.x2Reg != asm.NilRegister {
							compiler.assembler.CompileConstToRegister(amd64.MOVQ, int64(x2Value), tc.x2Reg)
							compiler.pushRuntimeValueLocationOnRegister(tc.x2Reg, runtimeValueTypeI64)
						} else {
							loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
							loc.valueType = runtimeValueTypeI64
							env.stack()[loc.stackPointer] = uint64(x2Value)
						}

						switch kind {
						case wazeroir.OperationKindDiv:
							err = compiler.compileDiv(wazeroir.NewOperationDiv(wazeroir.SignedTypeInt64))
						case wazeroir.OperationKindMul:
							err = compiler.compileMul(wazeroir.NewOperationMul(wazeroir.UnsignedTypeI64))
						case wazeroir.OperationKindRem:
							err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedUint64))
						}
						require.NoError(t, err)

						require.Equal(t, registerTypeGeneralPurpose, compiler.runtimeValueLocationStack().peek().getRegisterType())
						requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)
						require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
						// At this point, the previous value on the DX register is saved to the stack.
						require.True(t, prevOnDX.onStack())

						// We add the value previously on the DX with the multiplication result
						// in order to ensure that not saving existing DX value would cause
						// the failure in a subsequent instruction.
						err = compiler.compileAdd(wazeroir.NewOperationAdd(wazeroir.UnsignedTypeI64))
						require.NoError(t, err)

						require.NoError(t, compiler.compileReturnFunction())

						// Generate the code under test.
						code, _, err := compiler.compile()
						require.NoError(t, err)

						// Run code.
						env.exec(code)

						// Verify the stack is in the form of ["any value previously used by DX" + the result of operation]
						switch kind {
						case wazeroir.OperationKindDiv:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, uint64(x1Value/x2Value)+dxValue, env.stackTopAsUint64())
						case wazeroir.OperationKindMul:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, uint64(x1Value*x2Value)+dxValue, env.stackTopAsUint64())
						case wazeroir.OperationKindRem:
							require.Equal(t, uint64(1), env.stackPointer())
							require.Equal(t, x1Value%x2Value+dxValue, env.stackTopAsUint64())
						}
					})
				}
			})
		})
	}
}

func TestAmd64Compiler_readInstructionAddress(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Set the acquisition target instruction to the one after JMP.
		compiler.assembler.CompileReadInstructionAddress(amd64.RegAX, amd64.JMP)

		// If generate the code without JMP after readInstructionAddress,
		// the call back added must return error.
		_, _, err = compiler.compile()
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		env := newCompilerEnvironment()
		compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		const destinationRegister = amd64.RegAX
		// Set the acquisition target instruction to the one after RET,
		// and read the absolute address into destinationRegister.
		compiler.assembler.CompileReadInstructionAddress(destinationRegister, amd64.RET)

		// Jump to the instruction after RET below via the absolute
		// address stored in destinationRegister.
		compiler.assembler.CompileJumpToRegister(amd64.JMP, destinationRegister)

		compiler.assembler.CompileStandAlone(amd64.RET)

		// This could be the read instruction target as this is the
		// right after RET. Therefore, the jmp instruction above
		// must target here.
		const expectedReturnValue uint32 = 10000
		err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: expectedReturnValue})
		require.NoError(t, err)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		// Generate the code under test.
		code, _, err := compiler.compile()
		require.NoError(t, err)

		// Run code.
		env.exec(code)

		require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expectedReturnValue, env.stackTopAsUint32())
	})
}

func TestAmd64Compiler_preventCrossedTargetdRegisters(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newAmd64Compiler, nil).(*amd64Compiler)

	tests := []struct {
		initial           []*runtimeValueLocation
		desired, expected []asm.Register
	}{
		{
			initial:  []*runtimeValueLocation{{register: amd64.RegAX}, {register: amd64.RegCX}, {register: amd64.RegDX}},
			desired:  []asm.Register{amd64.RegDX, amd64.RegCX, amd64.RegAX},
			expected: []asm.Register{amd64.RegDX, amd64.RegCX, amd64.RegAX},
		},
		{
			initial:  []*runtimeValueLocation{{register: amd64.RegAX}, {register: amd64.RegCX}, {register: amd64.RegDX}},
			desired:  []asm.Register{amd64.RegDX, amd64.RegAX, amd64.RegCX},
			expected: []asm.Register{amd64.RegDX, amd64.RegAX, amd64.RegCX},
		},
		{
			initial:  []*runtimeValueLocation{{register: amd64.RegR8}, {register: amd64.RegR9}, {register: amd64.RegR10}},
			desired:  []asm.Register{amd64.RegR8, amd64.RegR9, amd64.RegR10},
			expected: []asm.Register{amd64.RegR8, amd64.RegR9, amd64.RegR10},
		},
		{
			initial:  []*runtimeValueLocation{{register: amd64.RegBX}, {register: amd64.RegDX}, {register: amd64.RegCX}},
			desired:  []asm.Register{amd64.RegR8, amd64.RegR9, amd64.RegR10},
			expected: []asm.Register{amd64.RegBX, amd64.RegDX, amd64.RegCX},
		},
		{
			initial:  []*runtimeValueLocation{{register: amd64.RegR8}, {register: amd64.RegR9}, {register: amd64.RegR10}},
			desired:  []asm.Register{amd64.RegAX, amd64.RegCX, amd64.RegR9},
			expected: []asm.Register{amd64.RegR8, amd64.RegR10, amd64.RegR9},
		},
	}

	for _, tt := range tests {
		initialRegisters := collectRegistersFromRuntimeValues(tt.initial)
		restoreCrossing := compiler.compilePreventCrossedTargetRegisters(tt.initial, tt.desired)
		// Required expected state after prevented crossing.
		require.Equal(t, tt.expected, collectRegistersFromRuntimeValues(tt.initial))
		restoreCrossing()
		// Require initial state after restoring.
		require.Equal(t, initialRegisters, collectRegistersFromRuntimeValues(tt.initial))
	}
}

// mockCpuFlags implements platform.CpuFeatureFlags
type mockCpuFlags struct {
	flags      uint64
	extraFlags uint64
}

// Has implements the method of the same name in platform.CpuFeatureFlags
func (f *mockCpuFlags) Has(flag uint64) bool {
	return (f.flags & flag) != 0
}

// HasExtra implements the method of the same name in platform.CpuFeatureFlags
func (f *mockCpuFlags) HasExtra(flag uint64) bool {
	return (f.extraFlags & flag) != 0
}

// Relates to #1111 (Clz): older AMD64 CPUs do not support the LZCNT instruction
// CPUID should be used instead. We simulate presence/absence of the feature
// by overriding the field in the corresponding struct.
func TestAmd64Compiler_ensureClz_ABM(t *testing.T) {
	tests := []struct {
		name         string
		cpuFeatures  platform.CpuFeatureFlags
		expectedCode string
	}{
		{
			name:         "with ABM",
			expectedCode: "b80a000000f3480fbdc0",
			cpuFeatures: &mockCpuFlags{
				flags:      0,
				extraFlags: platform.CpuExtraFeatureABM,
			},
		},
		{
			name:         "without ABM",
			expectedCode: "b80a0000004883f8007507b840000000eb08480fbdc04883f03f",
			cpuFeatures: &mockCpuFlags{
				flags:      0,
				extraFlags: 0, // no flags, thus no ABM, i.e. no LZCNT
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			newCompiler := func() compiler {
				c := newCompiler().(*amd64Compiler)
				// override auto-detected CPU features with the test case
				c.cpuFeatures = tt.cpuFeatures
				return c
			}

			compiler := env.requireNewCompiler(t, newCompiler, nil)

			err := compiler.compileConstI32(wazeroir.OperationConstI32{Value: 10})
			require.NoError(t, err)

			err = compiler.compileClz(wazeroir.NewOperationClz(wazeroir.UnsignedInt64))
			require.NoError(t, err)

			compiler.compileNOP() // pad for jump target (when no ABM)

			code, _, err := compiler.compile()
			require.NoError(t, err)

			require.Equal(t, tt.expectedCode, hex.EncodeToString(code))
		})
	}
}

// Relates to #1111 (Ctz): older AMD64 CPUs do not support the LZCNT instruction
// CPUID should be used instead. We simulate presence/absence of the feature
// by overriding the field in the corresponding struct.
func TestAmd64Compiler_ensureCtz_ABM(t *testing.T) {
	tests := []struct {
		name         string
		cpuFeatures  platform.CpuFeatureFlags
		expectedCode string
	}{
		{
			name:         "with ABM",
			expectedCode: "b80a000000f3480fbcc0",
			cpuFeatures: &mockCpuFlags{
				flags:      0,
				extraFlags: platform.CpuExtraFeatureABM,
			},
		},
		{
			name:         "without ABM",
			expectedCode: "b80a0000004883f8007507b840000000eb05f3480fbcc0",
			cpuFeatures: &mockCpuFlags{
				flags:      0,
				extraFlags: 0, // no flags, thus no ABM, i.e. no LZCNT
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			newCompiler := func() compiler {
				c := newCompiler().(*amd64Compiler)
				// override auto-detected CPU features with the test case
				c.cpuFeatures = tt.cpuFeatures
				return c
			}

			compiler := env.requireNewCompiler(t, newCompiler, nil)

			err := compiler.compileConstI32(wazeroir.OperationConstI32{Value: 10})
			require.NoError(t, err)

			err = compiler.compileCtz(wazeroir.NewOperationCtz(wazeroir.UnsignedInt64))
			require.NoError(t, err)

			compiler.compileNOP() // pad for jump target (when no ABM)

			code, _, err := compiler.compile()
			require.NoError(t, err)

			require.Equal(t, tt.expectedCode, hex.EncodeToString(code))
		})
	}
}

// collectRegistersFromRuntimeValues returns the registers occupied by locs.
func collectRegistersFromRuntimeValues(locs []*runtimeValueLocation) []asm.Register {
	out := make([]asm.Register, len(locs))
	for i := range locs {
		out[i] = locs[i].register
	}
	return out
}

// compile implements compilerImpl.getOnStackPointerCeilDeterminedCallBack for the amd64 architecture.
func (c *amd64Compiler) getOnStackPointerCeilDeterminedCallBack() func(uint64) {
	return c.onStackPointerCeilDeterminedCallBack
}

// compile implements compilerImpl.setStackPointerCeil for the amd64 architecture.
func (c *amd64Compiler) setStackPointerCeil(v uint64) {
	c.stackPointerCeil = v
}

// compile implements compilerImpl.setRuntimeValueLocationStack for the amd64 architecture.
func (c *amd64Compiler) setRuntimeValueLocationStack(s runtimeValueLocationStack) {
	c.locationStack = s
}
