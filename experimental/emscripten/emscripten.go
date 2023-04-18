package emscripten

import (
	"context"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/emscripten"
	internal "github.com/tetratelabs/wazero/internal/emscripten"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type emscriptenFns []*wasm.HostFunc

// InstantiateForModule instantiates a module named "env" populated with any
// known functions used in emscripten.
func InstantiateForModule(ctx context.Context, r wazero.Runtime, guest wazero.CompiledModule) (api.Closer, error) {
	// Create the exporter for the supplied wasm
	exporter, err := NewFunctionExporterForModule(guest)
	if err != nil {
		return nil, err
	}

	// Instantiate it!
	env := r.NewHostModuleBuilder("env")
	exporter.ExportFunctions(env)
	return env.Instantiate(ctx)
}

// NewFunctionExporterForModule returns a guest-specific FunctionExporter,
// populated with any known functions used in emscripten.
func NewFunctionExporterForModule(guest wazero.CompiledModule) (emscripten.FunctionExporter, error) {
	ret := emscriptenFns{}
	for _, fn := range guest.ImportedFunctions() {
		importModule, importName, isImport := fn.Import()
		if !isImport || importModule != "env" {
			continue // not emscripten
		}
		if importName == internal.FunctionNotifyMemoryGrowth {
			ret = append(ret, internal.NotifyMemoryGrowth)
			continue
		}
		if !strings.HasPrefix(importName, internal.InvokePrefix) {
			continue // not invoke, and maybe not emscripten
		}

		hf := internal.NewInvokeFunc(importName, fn.ParamTypes(), fn.ResultTypes())
		ret = append(ret, hf)
	}
	return ret, nil
}

// ExportFunctions implements FunctionExporter.ExportFunctions
func (i emscriptenFns) ExportFunctions(builder wazero.HostModuleBuilder) {
	exporter := builder.(wasm.HostFuncExporter)
	for _, fn := range i {
		exporter.ExportHostFunc(fn)
	}
}
