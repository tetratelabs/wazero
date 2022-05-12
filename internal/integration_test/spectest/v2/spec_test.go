package spectest

import (
	"embed"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS //nolint:unused

const enabledFeatures = wasm.Features20220419

func TestJIT(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}

	spectest.Run(t, testcases, jit.NewEngine, enabledFeatures)
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, testcases, interpreter.NewEngine, enabledFeatures)
}
