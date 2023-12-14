package opt

import (
	"runtime"

	"github.com/tetratelabs/wazero"
)

type enabler interface {
	// EnableOptimizingCompiler enables the optimizing compiler.
	// This is only implemented the internal type of wazero.runtimeConfig.
	EnableOptimizingCompiler()
}

// NewRuntimeConfigOptimizingCompiler returns a new RuntimeConfig with the optimizing compiler enabled.
func NewRuntimeConfigOptimizingCompiler() wazero.RuntimeConfig {
	if runtime.GOARCH != "arm64" {
		panic("UseOptimizingCompiler is only supported on arm64")
	}
	c := wazero.NewRuntimeConfig()
	c.(enabler).EnableOptimizingCompiler()
	return c
}
