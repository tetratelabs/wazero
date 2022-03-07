package post1_0

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestJIT(t *testing.T) {
	if !wazero.JITSupported {
		t.Skip()
	}
	runPostMVPTests(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreter(t *testing.T) {
	runPostMVPTests(t, wazero.NewRuntimeConfigInterpreter)
}

// runPostMVPTests tests features
// See https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
func runPostMVPTests(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	// TODO:
}
