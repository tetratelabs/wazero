package spectest

import (
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

const enabledFeatures = wasm.Features20191205

func TestJIT(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}

	Run(t, testcases, jit.NewEngine, enabledFeatures)
}

func TestInterpreter(t *testing.T) {
	Run(t, testcases, interpreter.NewEngine, enabledFeatures)
}
