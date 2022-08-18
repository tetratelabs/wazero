package compilationcache

import (
	"crypto/sha256"
	"io"
)

// Cache is the interface for compilation caches. Internally, the cache
// here means that the compiled binary cache by compilers. Regardless of the usage of
// ExternCache, the compiled functions are cached in memory, but its lifetime is
// bound to the lifetime of wazero.Runtime or wazero.CompiledModule.
// Usually, the compilation of Wasm binary is time-consuming. Therefore, you might
// want to cache the compilation result across the processes of wazero users.
//
// Since these methods are concurrently accessed, the implementations must be Goroutine-safe.
//
// See NewFileCache for the example implementation.
type Cache interface {
	// Get is called when the runtime is trying to get the cached content.
	// Implementations are supposed to return `content` which can be used to
	// read the content passed by Add as-is. Returns ok=true if the
	// content was found on the cache. That means the content is not empty
	// if and only if ok=true. In the case of not-found, this should return
	// ok=false with err=nil. content.Close() is automatically called by
	// the caller of this Get.
	//
	// Note: the returned content won't go through the validation pass of Wasm binary
	// which is applied when the binary is compiled from scratch without cache hit.
	// Its implication is that the implementors of ExternCache might want to have
	// their own validation phases. For example, sign the binary passed to Add, and
	// verify the signature of the stored cache before returning it via Get, etc.
	Get(key Key) (content io.ReadCloser, ok bool, err error)
	// Add is called when the runtime is trying to add the new cache entry.
	// The given `content` must be un-modified, and returned as-is in Get method.
	//
	// Note: the `content` is ensured to be safe through the validation phase applied on the Wasm binary.
	Add(key Key, content io.Reader) (err error)
	// Delete is called when the cache on the `key` returned by Get is no longer usable, and
	// must be purged. Specifically, this is called happens when the wazero's version has been changed.
	// For example, that is when there's a difference between the version of compiling wazero and the
	// version of the currently used wazero.
	Delete(key Key) (err error)
}

// Key represents the 256-bit unique identifier assigned to each cache content.
type Key = [sha256.Size]byte
