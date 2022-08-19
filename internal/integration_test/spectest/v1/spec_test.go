package v1

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/engine/interpreter"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/platform"
)

func TestCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	spectest.Run(t, Testcases, context.Background(), compiler.NewEngine, EnabledFeatures)
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, Testcases, context.Background(), interpreter.NewEngine, EnabledFeatures)
}

func TestBinaryEncoder(t *testing.T) {
	spectest.TestBinaryEncoder(t, Testcases, EnabledFeatures)
}
