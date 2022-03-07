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
	runOptionalFeatureTests(t, wazero.NewRuntimeConfigJIT)
}

func TestInterpreter(t *testing.T) {
	runOptionalFeatureTests(t, wazero.NewRuntimeConfigInterpreter)
}

// runOptionalFeatureTests tests features enabled by feature flags (internalwasm.Features) as they were unfinished when
// WebAssembly 1.0 (20191205) was released.
//
// See https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
func runOptionalFeatureTests(t *testing.T, newRuntimeConfig func() *wazero.RuntimeConfig) {
	// TODO:
}
