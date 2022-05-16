package spectest

import (
	"embed"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/engine/interpreter"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS

const enabledFeatures = wasm.Features20191205

func TestCompiler(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}

	spectest.Run(t, testcases, compiler.NewEngine, enabledFeatures, func(string) bool { return true })
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, testcases, interpreter.NewEngine, enabledFeatures, func(jsonname string) bool { return true })
}

func TestBinaryEncoder(t *testing.T) {
	spectest.TestBinaryEncoder(t, testcases, enabledFeatures)
}
