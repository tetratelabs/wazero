package wazero_test

import (
	"context"
	_ "embed"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// This is a basic example of retrieving custom sections using RuntimeConfig.WithCustomSections.
func Example_runtimeConfig_WithCustomSections() {
	ctx := context.Background()
	config := wazero.NewRuntimeConfig().WithCustomSections(true)

	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(ctx)

	m, err := r.CompileModule(ctx, addWasm)
	if err != nil {
		log.Panicln(err)
	}

	if m.CustomSections() == nil {
		log.Panicln("Custom sections should not be nil")
	}

	mustContain(m.CustomSections(), "producers")
	mustContain(m.CustomSections(), "target_features")

	// Output:
	//
}

func mustContain(ss []api.CustomSection, name string) {
	for _, s := range ss {
		if s.Name() == name {
			return
		}
	}
	log.Panicf("Could not find section named %s\n", name)
}
