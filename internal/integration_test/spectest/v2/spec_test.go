package spectest

import (
	"embed"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/engine/interpreter"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS //nolint:unused

const enabledFeatures = wasm.Features20220419

func TestCompiler(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}

	spectest.Run(t, testcases, compiler.NewEngine, enabledFeatures, func(jsonname string) bool {
		// TODO: remove after SIMD proposal
		if strings.Contains(jsonname, "simd") {
			return path.Base(jsonname) == "simd_const.json"
		}
		return true
	})
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, testcases, interpreter.NewEngine, enabledFeatures, func(jsonname string) bool {
		// TODO: remove after SIMD proposal
		if strings.Contains(jsonname, "simd") {
			return path.Base(jsonname) == "simd_const.json"
		}
		return true
	})
}
