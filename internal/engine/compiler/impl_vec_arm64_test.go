package compiler

import (
	"encoding/binary"
	"testing"

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
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	env.exec(code)

	lo, hi := env.stackTopAsV128()
	var actual [16]byte
	binary.LittleEndian.PutUint64(actual[:8], lo)
	binary.LittleEndian.PutUint64(actual[8:], hi)
	require.Equal(t, exp, actual)
}
