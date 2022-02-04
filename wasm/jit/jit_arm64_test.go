//go:build arm64
// +build arm64

package jit

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/wasm"
)

func (j *jitEnv) requireNewCompiler(t *testing.T) *arm64Compiler {
	cmp, err := newCompiler(&wasm.FunctionInstance{ModuleInstance: j.moduleInstance}, nil)
	require.NoError(t, err)
	return cmp.(*arm64Compiler)
}

func Test_return(t *testing.T) {
	env := newJITEnvironment()

	// Build codes.
	compiler := env.requireNewCompiler(t)
	err := compiler.emitPreamble()
	require.NoError(t, err)
	compiler.exit(jitCallStatusCodeReturned)

	// Generate the code under test.
	code, _, _, err := compiler.generate()
	require.NoError(t, err)

	fmt.Println(hex.EncodeToString(code))

	// Run codes
	env.exec(code)

	// JIT status on engine must be updated.
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}
