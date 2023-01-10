// Package run exists to avoid dependency cycles when keeping most of gojs
// code internal.
package run

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/gojs"
)

func RunAndReturnState(ctx context.Context, r wazero.Runtime, compiled wazero.CompiledModule, config wazero.ModuleConfig) (*gojs.State, error) {
	// Instantiate the module compiled by go, noting it has no init function.

	mod, err := r.InstantiateModule(ctx, compiled, config)
	if err != nil {
		return nil, err
	}
	defer mod.Close(ctx)

	// Extract the args and env from the module config and write it to memory.
	argc, argv, err := gojs.WriteArgsAndEnviron(mod)
	if err != nil {
		return nil, err
	}

	// Create host-side state for JavaScript values and events.
	s := gojs.NewState(ctx)
	ctx = context.WithValue(ctx, gojs.StateKey{}, s)

	// Invoke the run function.
	_, err = mod.ExportedFunction("run").Call(ctx, uint64(argc), uint64(argv))
	return s, err
}
