package compiler

import (
	"encoding/binary"
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// TestArm64Compiler_V128Shuffle_ConstTable_MiddleOfFunction ensures that flushing constant table in the middle of
// function works well by intentionally setting arm64.AssemblerImpl MaxDisplacementForConstantPool = 0.
func TestArm64Compiler_V128Shuffle_ConstTable_MiddleOfFunction(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler,
		&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	lanes := [16]byte{1, 1, 1, 1, 0, 0, 0, 0, 10, 10, 10, 10, 0, 0, 0, 0}
	v := [16]byte{0: 0xa, 1: 0xb, 10: 0xc}
	w := [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	exp := [16]byte{
		0xb, 0xb, 0xb, 0xb,
		0xa, 0xa, 0xa, 0xa,
		0xc, 0xc, 0xc, 0xc,
		0xa, 0xa, 0xa, 0xa,
	}

	err = compiler.compileV128Const(&wazeroir.OperationV128Const{
		Lo: binary.LittleEndian.Uint64(v[:8]),
		Hi: binary.LittleEndian.Uint64(v[8:]),
	})
	require.NoError(t, err)

	err = compiler.compileV128Const(&wazeroir.OperationV128Const{
		Lo: binary.LittleEndian.Uint64(w[:8]),
		Hi: binary.LittleEndian.Uint64(w[8:]),
	})
	require.NoError(t, err)

	err = compiler.compileV128Shuffle(&wazeroir.OperationV128Shuffle{Lanes: lanes})
	require.NoError(t, err)

	assembler := compiler.(*arm64Compiler).assembler.(*arm64.AssemblerImpl)
	assembler.MaxDisplacementForConstantPool = 0 // Ensures that constant table for shuffle will be flushed immediately.

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)

	env.exec(code)

	lo, hi := env.stackTopAsV128()
	var actual [16]byte
	binary.LittleEndian.PutUint64(actual[:8], lo)
	binary.LittleEndian.PutUint64(actual[8:], hi)
	require.Equal(t, exp, actual)
}

func TestArm64Compiler_V128Shuffle_combinations(t *testing.T) {
	movValueRegisterToRegister := func(t *testing.T, c *arm64Compiler, src *runtimeValueLocation, dst asm.Register) {
		c.assembler.CompileTwoVectorRegistersToVectorRegister(arm64.VORR, src.register, src.register, dst,
			arm64.VectorArrangement16B)
		c.locationStack.markRegisterUnused(src.register)
		src.setRegister(dst)
		// We have to set the lower 64-bits' location as well.
		c.locationStack.stack[src.stackPointer-1].setRegister(dst)
		c.locationStack.markRegisterUsed(dst)
	}

	tests := []struct {
		name                        string
		init                        func(t *testing.T, c *arm64Compiler)
		wReg, vReg                  asm.Register
		verifyFnc                   func(t *testing.T, env *compilerEnv)
		expStackPointerAfterShuffle uint64
	}{
		{
			name:                        "w=v1, v=v2",
			wReg:                        arm64.RegV1,
			vReg:                        arm64.RegV2,
			init:                        func(t *testing.T, c *arm64Compiler) {},
			verifyFnc:                   func(t *testing.T, env *compilerEnv) {},
			expStackPointerAfterShuffle: 2,
		},
		{
			name:                        "w=v2, v=v1",
			wReg:                        arm64.RegV2,
			vReg:                        arm64.RegV1,
			init:                        func(t *testing.T, c *arm64Compiler) {},
			verifyFnc:                   func(t *testing.T, env *compilerEnv) {},
			expStackPointerAfterShuffle: 2,
		},
		{
			name:                        "w=v29, v=v30",
			wReg:                        arm64.RegV29, // will be moved to v30.
			vReg:                        arm64.RegV30, // will be moved to v29.
			init:                        func(t *testing.T, c *arm64Compiler) {},
			verifyFnc:                   func(t *testing.T, env *compilerEnv) {},
			expStackPointerAfterShuffle: 2,
		},
		{
			name: "w=v12, v=v30",
			wReg: arm64.RegV12, // will be moved to v30.
			vReg: arm64.RegV30, // will be moved to v29.
			init: func(t *testing.T, c *arm64Compiler) {
				// Set up the previous value on the v3 register.
				err := c.compileV128Const(&wazeroir.OperationV128Const{
					Lo: 1234,
					Hi: 5678,
				})
				require.NoError(t, err)
				movValueRegisterToRegister(t, c, c.locationStack.peek(), arm64.RegV29)
			},
			verifyFnc: func(t *testing.T, env *compilerEnv) {
				// Previous value on the V3 register must be saved onto the stack.
				lo, hi := env.stack()[0], env.stack()[1]
				require.Equal(t, uint64(1234), lo)
				require.Equal(t, uint64(5678), hi)
			},
			expStackPointerAfterShuffle: 4,
		},
		{
			name: "w=v29, v=v12",
			wReg: arm64.RegV29, // will be moved to v30.
			vReg: arm64.RegV12, // will be moved to v29.
			init: func(t *testing.T, c *arm64Compiler) {
				// Set up the previous value on the v3 register.
				err := c.compileV128Const(&wazeroir.OperationV128Const{
					Lo: 1234,
					Hi: 5678,
				})
				require.NoError(t, err)
				movValueRegisterToRegister(t, c, c.locationStack.peek(), arm64.RegV30)
			},
			verifyFnc: func(t *testing.T, env *compilerEnv) {
				// Previous value on the V3 register must be saved onto the stack.
				lo, hi := env.stack()[0], env.stack()[1]
				require.Equal(t, uint64(1234), lo)
				require.Equal(t, uint64(5678), hi)
			},
			expStackPointerAfterShuffle: 4,
		},
	}

	lanes := [16]byte{1, 1, 1, 1, 0, 0, 0, 0, 10, 10, 10, 10, 0, 0, 0, 31}
	v := [16]byte{0: 0xa, 1: 0xb, 10: 0xc}
	w := [16]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1}
	exp := [16]byte{
		0xb, 0xb, 0xb, 0xb,
		0xa, 0xa, 0xa, 0xa,
		0xc, 0xc, 0xc, 0xc,
		0xa, 0xa, 0xa, 1,
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler,
				&wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			ac := compiler.(*arm64Compiler)
			tc.init(t, ac)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(v[:8]),
				Hi: binary.LittleEndian.Uint64(v[8:]),
			})
			require.NoError(t, err)

			vLocation := compiler.runtimeValueLocationStack().peek()
			movValueRegisterToRegister(t, ac, vLocation, tc.vReg)

			err = compiler.compileV128Const(&wazeroir.OperationV128Const{
				Lo: binary.LittleEndian.Uint64(w[:8]),
				Hi: binary.LittleEndian.Uint64(w[8:]),
			})
			require.NoError(t, err)

			wLocation := compiler.runtimeValueLocationStack().peek()
			movValueRegisterToRegister(t, ac, wLocation, tc.wReg)

			err = compiler.compileV128Shuffle(&wazeroir.OperationV128Shuffle{Lanes: lanes})
			require.NoError(t, err)

			require.Equal(t, tc.expStackPointerAfterShuffle, compiler.runtimeValueLocationStack().sp)
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			env.exec(code)

			lo, hi := env.stackTopAsV128()
			var actual [16]byte
			binary.LittleEndian.PutUint64(actual[:8], lo)
			binary.LittleEndian.PutUint64(actual[8:], hi)
			require.Equal(t, exp, actual)

			tc.verifyFnc(t, env)
		})
	}
}
