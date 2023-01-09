package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// NewCache returns a new Cache to be passed to RuntimeConfig.
func NewCache() Cache {
	return &cache{}
}

// cache implements Cache interface.
type cache struct {
	// eng is the engine for this cache. If the cache is configured, the engine is shared across multiple instances of
	// Runtime, and its lifetime is not bound to them. Instead, the engine is alive until Cache.Close is called.
	eng wasm.Engine

	// TODO: move the experimental file cache configuration here.
}

// Close implements the same method on the Cache interface.
func (c *cache) Close(_ context.Context) (err error) {
	if c.eng != nil {
		err = c.eng.Close()
	}
	return
}
