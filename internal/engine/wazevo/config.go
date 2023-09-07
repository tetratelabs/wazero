package wazevo

import (
	"context"
	"unsafe"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// ConfigureWazevo modifies wazero.RuntimeConfig and sets the wazevo implementation.
// This is a hack to avoid modifying outside the wazevo package while testing it end-to-end.
//
// Until we expose it in the experimental public API, use this for internal testing.
func ConfigureWazevo(config wazero.RuntimeConfig) {
	// This is the internal representation of interface in Go.
	// https://research.swtch.com/interfaces
	type iface struct {
		_    *byte
		data unsafe.Pointer
	}

	configInterface := (*iface)(unsafe.Pointer(&config))

	// This corresponds to the unexported wazero.runtimeConfig, and the target field newEngine exists
	// in the middle of the implementation.
	type newEngine func(context.Context, api.CoreFeatures, filecache.Cache) wasm.Engine
	type runtimeConfig struct {
		enabledFeatures       api.CoreFeatures
		memoryLimitPages      uint32
		memoryCapacityFromMax bool
		engineKind            int
		dwarfDisabled         bool
		newEngine
		// Other fields follow, but we don't care.
	}
	cm := (*runtimeConfig)(configInterface.data)
	// Insert the wazevo implementation.
	cm.newEngine = NewEngine
}
