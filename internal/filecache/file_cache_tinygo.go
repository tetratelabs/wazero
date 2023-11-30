//go:build tinygo

package filecache

import (
	"io"
)

// New returns a new Cache implemented by an in-memory cache. Possibly Flash memory...
func New(dir string) Cache {
	return newMemoryCache()
}

func newMemoryCache() *memoryCache {
	return &memoryCache{}
}

// memoryCache persists compiled functions into memory.
type memoryCache struct {
}

func (mc *memoryCache) Get(key Key) (content io.ReadCloser, ok bool, err error) {
	return nil, false, nil
}

func (mc *memoryCache) Add(key Key, content io.Reader) (err error) {
	return
}

func (mc *memoryCache) Delete(key Key) (err error) {
	return
}
