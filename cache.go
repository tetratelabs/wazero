package wazero

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"sync"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// CompilationCache reduces time spent compiling (Runtime.CompileModule) the same wasm module.
//
// # Notes
//
//   - This is an interface for decoupling, not third-party implementations.
//     All implementations are in wazero.
//   - Instances of this can be reused across multiple runtimes, if configured
//     via RuntimeConfig.
//   - The cache check happens before the compilation, so if multiple Goroutines are
//     trying to compile the same module simultaneously, it is possible that they
//     all compile the module. The design here is that the lock isn't held for the action "Compile"
//     but only for checking and saving the compiled result. Therefore, we strongly recommend that the embedder
//     does the centralized compilation in a single Goroutines (or multiple Goroutines per Wasm binary) to generate cache rather than
//     trying to Compile in parallel for a single module. In other words, we always recommend to produce CompiledModule
//     share it across multiple Goroutines to avoid trying to compile the same module simultaneously.
type CompilationCache interface{ api.Closer }

// NewCompilationCache returns a new CompilationCache to be passed to RuntimeConfig.
// This configures only in-memory cache, and doesn't persist to the file system. See wazero.NewCompilationCacheWithDir for detail.
//
// The returned CompilationCache can be used to share the in-memory compilation results across multiple instances of wazero.Runtime.
func NewCompilationCache() CompilationCache {
	return &cache{}
}

// NewCompilationCacheWithDir is like wazero.NewCompilationCache except the result also writes
// state into the directory specified by `dirname` parameter.
//
// If the dirname doesn't exist, this creates it or returns an error.
//
// Those running wazero as a CLI or frequently restarting a process using the same wasm should
// use this feature to reduce time waiting to compile the same module a second time.
//
// The contents written into dirname are wazero-version specific, meaning different versions of
// wazero will duplicate entries for the same input wasm.
//
// Note: The embedder must safeguard this directory from external changes.
func NewCompilationCacheWithDir(dirname string) (CompilationCache, error) {
	c := &cache{}
	err := c.ensuresFileCache(dirname, version.GetWazeroVersion())
	return c, err
}

// Compilation cache errors can be classified with errors.Is.
var (
	// ErrCompilationCacheMiss indicates that a required entry is absent.
	ErrCompilationCacheMiss = filecache.ErrMiss
	// ErrCompilationCacheStale indicates an incompatible entry.
	ErrCompilationCacheStale = filecache.ErrStale
	// ErrCompilationCacheCorrupt indicates that an entry could not be decoded.
	ErrCompilationCacheCorrupt = filecache.ErrCorrupt
	// ErrCompilationCacheIO indicates a directory or entry access failure.
	ErrCompilationCacheIO = filecache.ErrIO
	// ErrCompilationCacheUnsupported indicates an engine without persistent cache support.
	ErrCompilationCacheUnsupported = filecache.ErrUnsupported
)

// NewCompilationCacheWithDirReadOnly opens an existing persistent compilation cache.
// Both dirname and its version/OS/architecture subdirectory must already exist.
// Unlike NewCompilationCacheWithDir, this never creates, writes, or deletes files.
// Guest modules must hit the in-memory or persistent cache: missing, stale, or
// corrupt entries return classifiable errors without compiling the guest module.
// The interpreter (including automatic fallback on unsupported platforms) rejects
// guest modules with ErrCompilationCacheUnsupported. Host modules are permitted.
//
// This is not a zero-code-generation mode: shared/host trampolines and bounded
// per-type entry preambles for cache hits can still be generated.
// Cache entries are trusted executable input, not a safe untrusted format. The
// embedder must protect the directory and use the same wazero version, CPU
// features, and compilation configuration when warming and reading the cache.
func NewCompilationCacheWithDirReadOnly(dirname string) (CompilationCache, error) {
	c := &cache{readOnly: true}
	if err := c.ensuresFileCache(dirname, version.GetWazeroVersion()); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCompilationCacheIO, err)
	}
	return c, nil
}

// cache implements Cache interface.
type cache struct {
	// eng is the engine for this cache. If the cache is configured, the engine is shared across multiple instances of
	// Runtime, and its lifetime is not bound to them. Instead, the engine is alive until Cache.Close is called.
	engs      [engineKindCount]wasm.Engine
	fileCache filecache.Cache
	readOnly  bool
	initOnces [engineKindCount]sync.Once
}

func (c *cache) initEngine(ek engineKind, ne newEngine, ctx context.Context, features api.CoreFeatures) wasm.Engine {
	c.initOnces[ek].Do(func() { c.engs[ek] = ne(ctx, features, c.fileCache) })
	return c.engs[ek]
}

// Close implements the same method on the Cache interface.
func (c *cache) Close(_ context.Context) (err error) {
	for _, eng := range c.engs {
		if eng != nil {
			if err = eng.Close(); err != nil {
				return
			}
		}
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

	ensureDir := mkdir
	if c.readOnly {
		ensureDir = existingDir
	}
	// Ensure the user-supplied directory.
	if err = ensureDir(dir); err != nil {
		return err
	}

	// Create a version-specific directory to avoid conflicts.
	dirname := path.Join(dir, "wazero-"+wazeroVersion+"-"+goruntime.GOARCH+"-"+goruntime.GOOS)
	if err = ensureDir(dirname); err != nil {
		return err
	}

	if c.readOnly {
		c.fileCache = filecache.NewReadOnly(dirname)
	} else {
		c.fileCache = filecache.New(dirname)
	}
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

func existingDir(dirname string) error {
	st, err := os.Stat(dirname)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not dir", dirname)
	}
	return nil
}
