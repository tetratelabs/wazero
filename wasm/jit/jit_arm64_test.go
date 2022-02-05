//go:build arm64
// +build arm64

package jit

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/wasm"
)

func (j *jitEnv) requireNewCompiler(t *testing.T) *arm64Compiler {
	cmp, err := newCompiler(&wasm.FunctionInstance{ModuleInstance: j.moduleInstance}, nil)
	require.NoError(t, err)
	return cmp.(*arm64Compiler)
}

func TestArchContextOffsetInEngine(t *testing.T) {
	var eng engine
	// If this fails, we have to fix jit_arm64.s as well.
	require.Equal(t, int(unsafe.Offsetof(eng.returnAddress)), engineArchContextReturnAddressOffset)
}

func Test_exit(t *testing.T) {
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
			require.NoError(t, err)
			compiler.exit(s)

			// Generate the code under test.
			code, _, _, err := compiler.generate()
			require.NoError(t, err)

			// Run codes
			env.exec(code)

			// JIT status on engine must be updated.
			require.Equal(t, s, env.jitStatus())
		})
	}
}
