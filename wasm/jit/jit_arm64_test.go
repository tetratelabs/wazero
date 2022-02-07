//go:build arm64
// +build arm64

package jit

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

func (j *jitEnv) requireNewCompiler(t *testing.T) *arm64Compiler {
	cmp, err := newCompiler(&wasm.FunctionInstance{ModuleInstance: j.moduleInstance}, nil)
	require.NoError(t, err)
	ret, ok := cmp.(*arm64Compiler)
	require.True(t, ok)
	return ret
}

func TestArm64CompilerEndToEnd(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "empty", body: []byte{wasm.OpcodeEnd}},
		{name: "br .return", body: []byte{wasm.OpcodeBr, 0, wasm.OpcodeEnd}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			engine := newEngine()
			f := &wasm.FunctionInstance{
				FunctionType: &wasm.TypeInstance{Type: &wasm.FunctionType{}},
				Body:         tc.body,
			}
			err := engine.Compile(f)
			require.NoError(t, err)
			_, err = engine.Call(ctx, f)
			require.NoError(t, err)
		})
	}
}

func TestArchContextOffsetInEngine(t *testing.T) {
	var eng engine
	// If this fails, we have to fix jit_arm64.s as well.
	require.Equal(t, int(unsafe.Offsetof(eng.jitCallReturnAddress)), engineArchContextJITCallReturnAddressOffset)
}

func TestArm64Compiler_returnFunction(t *testing.T) {
	env := newJITEnvironment()

	// Build codes.
	compiler := env.requireNewCompiler(t)
	err := compiler.emitPreamble()
	require.NoError(t, err)
	compiler.returnFunction()

	// Generate the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)

	// Run native code.
	env.exec(code)

	// JIT status on engine must be returned.
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	// Plus, the call frame stack pointer must be zero after return.
	require.Equal(t, uint64(0), env.callFrameStackPointer())
}

func TestArm64Compiler_exit(t *testing.T) {
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
			err := compiler.emitPreamble()

			expStackPointer := uint64(100)
			compiler.locationStack.sp = expStackPointer
			require.NoError(t, err)
			compiler.exit(s)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run codes
			env.exec(code)

			// JIT status on engine must be updated.
			require.Equal(t, s, env.jitStatus())

			// Stack pointer must be written on engine.stackPointer on return.
			require.Equal(t, expStackPointer, env.stackPointer())
		})
	}
}

func TestArm64Compiler_compileConsts(t *testing.T) {
	for _, op := range []wazeroir.OperationKind{
		// wazeroir.OperationKindConstI32,
		wazeroir.OperationKindConstI64,
		// wazeroir.OperationKindConstF32,
		// wazeroir.OperationKindConstF64,
	} {
		op := op
		t.Run(op.String(), func(t *testing.T) {
			for _, val := range []uint64{
				0,
			} {
				t.Run(fmt.Sprintf("0x%x", val), func(t *testing.T) {
					env := newJITEnvironment()

					// Build codes.
					compiler := env.requireNewCompiler(t)
					err := compiler.emitPreamble()
					require.NoError(t, err)

					switch op {
					case wazeroir.OperationKindConstI32:
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(val)})
					case wazeroir.OperationKindConstI64:
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
					case wazeroir.OperationKindConstF32:
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(val))})
					case wazeroir.OperationKindConstF64:
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
					}
					require.NoError(t, err)

					loc := compiler.locationStack.peek()
					require.True(t, loc.onRegister())

					compiler.releaseRegisterToStack(loc)
					compiler.returnFunction()

					// Generate the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					fmt.Println(hex.EncodeToString(code))
					// Run native code.
					env.exec(code)

					// JIT status on engine must be returned.
					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())

					switch op {
					case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstF32:
						require.Equal(t, uint32(val), env.stackTopAsUint32())
					case wazeroir.OperationKindConstI64, wazeroir.OperationKindConstF64:
						require.Equal(t, val, env.stackTopAsUint64())
					}
				})
			}
		})
	}
}
