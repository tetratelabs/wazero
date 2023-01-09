package wazero

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/compilationcache"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Cache is the configuration for caching behavior across
type Cache interface {
	api.Closer

	// WithCompilationCacheDirName configures the destination directory of the compilation cache.
	// Regardless of the usage of this, the compiled functions are cached in memory, but its lifetime is
	// bound to the lifetime of wazero.Runtime or wazero.CompiledModule.
	//
	// If the dirname doesn't exist, this creates the directory.
	//
	// With the given non-empty directory, wazero persists the cache into the directory and that cache
	// will be used as long as the running wazero version match the version of compilation wazero.
	//
	// A cache is only valid for use in one wazero.Runtime at a time. Concurrent use
	// of a wazero.Runtime is supported, but multiple runtimes must not share the
	// same directory.
	//
	// Note: The embedder must safeguard this directory from external changes.
	//
	// Usage:
	//
	//	ctx, _ := experimental.WithCompilationCacheDirName(context.Background(), "/home/me/.cache/wazero")
	//	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	WithCompilationCacheDirName(dir string) error
}

// NewCache returns a new Cache to be passed to RuntimeConfig.
func NewCache() Cache {
	return &cache{}
}

// cache implements Cache interface.
type cache struct {
	// eng is the engine for this cache. If the cache is configured, the engine is shared across multiple instances of
	// Runtime, and its lifetime is not bound to them. Instead, the engine is alive until Cache.Close is called.
	eng wasm.Engine

	fileCache compilationcache.Cache
}

// Close implements the same method on the Cache interface.
func (c *cache) Close(_ context.Context) (err error) {
	if c.eng != nil {
		err = c.eng.Close()
	}
	return
}

// WithCompilationCacheDirName implements the same method on the Cache interface.
func (c *cache) WithCompilationCacheDirName(dir string) error {
	return c.withCompilationCacheDirName(dir, version.GetWazeroVersion())
}

// WithCompilationCacheDirName implements the same method on the Cache interface.
func (c *cache) withCompilationCacheDirName(dir string, wazeroVersion string) error {
	// Resolve a potentially relative directory into an absolute one.
	var err error
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}

	// Ensure the user-supplied directory.
	if err = mkdir(dir); err != nil {
		return err
	}

	// Create a version-specific directory to avoid conflicts.
	dirname := path.Join(dir, "wazero-"+wazeroVersion+"-"+goruntime.GOARCH+"-"+goruntime.GOOS)
	if err = mkdir(dirname); err != nil {
		return err
	}

	c.fileCache = compilationcache.NewFileCache(dirname)
	return nil
}

func mkdir(dirname string) error {
	if st, err := os.Stat(dirname); errors.Is(err, os.ErrNotExist) {
		// If the directory not found, create the cache dir.
		if err = os.MkdirAll(dirname, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %v", dirname, err)
		}
	} else if err != nil {
		return err
	} else if !st.IsDir() {
		return fmt.Errorf("%s is not dir", dirname)
	}
	return nil
}
