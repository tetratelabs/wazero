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

// CompileCache is the configuration for compilation cache behavior of wazero.Runtime and can be passed to wazero.RuntimeConfig.
type CompileCache interface{ api.Closer }

// NewCache returns a new Cache to be passed to RuntimeConfig.
// This configures only in-memory cache, and doesn't persist to the file system. See wazero.NewCompileCacheWithDir for detail.
//
// The returned CompileCache can be used to share the in-memory compilation results across multiple instances of wazero.Runtime.
func NewCache() CompileCache {
	return &cache{}
}

// NewCompileCacheWithDir is the same as Cache returned by wazero.NewCache except that this also persists
// the compilation results into the directory specified by `dirname` parameter.
//
// If the dirname doesn't exist, this creates the directory. And if the directory creation fails, this returns an error.
//
// If the given directory already exists, and it's a file, this returns an error.
//
// With the given non-empty directory, wazero persists the cache into the directory and that cache
// will be used as long as the running wazero version match the version of compilation wazero.
//
// Note: The embedder must safeguard this directory from external changes.
func NewCompileCacheWithDir(dirname string) (CompileCache, error) {
	c := &cache{}
	err := c.ensuresFileCache(dirname, version.GetWazeroVersion())
	return c, err
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

func (c *cache) ensuresFileCache(dir string, wazeroVersion string) error {
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
