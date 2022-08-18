package compilationcache

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"sync"
)

// FileCachePathKey is a context.Context Value key. It allows overriding fs.FS for WASI.
type FileCachePathKey struct{}

// NewFileCache returns a new Cache implemented by fileCache.
func NewFileCache(ctx context.Context) Cache {
	if fsValue := ctx.Value(FileCachePathKey{}); fsValue != nil {
		return newFileCache(fsValue.(string))
	}
	return nil
}

func newFileCache(dir string) *fileCache {
	return &fileCache{dirPath: dir}
}

// fileCache is an example implementation of Cache which writes/reads cache into/from the fileCache.dirPath.
//
// Note: this can be expanded to do binary signing/verification, set TTL on each entry, etc.
type fileCache struct {
	dirPath string
	mux     sync.RWMutex
}

type fileReadCloser struct {
	*os.File
	fc *fileCache
}

func (fc *fileCache) path(key Key) string {
	return path.Join(fc.dirPath, hex.EncodeToString(key[:]))
}

func (fc *fileCache) Get(key Key) (content io.ReadCloser, ok bool, err error) {
	// TODO: take lock per key for more efficiency vs the complexity of impl.
	// Unlock is done inside the content.Close().
	fc.mux.RLock()
	f, err := os.Open(fc.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	} else {
		return &fileReadCloser{File: f, fc: fc}, true, nil
	}
}

// Close wraps the os.File Close to release the read lock on fileCache.
func (f *fileReadCloser) Close() (err error) {
	defer f.fc.mux.RUnlock()
	err = f.File.Close()
	return
}

func (fc *fileCache) Add(key Key, content io.Reader) (err error) {
	// TODO: take lock per key for more efficiency vs the complexity of impl.
	fc.mux.Lock()
	defer fc.mux.Unlock()

	file, err := os.Create(fc.path(key))
	if err != nil {
		return
	}
	defer file.Close()
	_, err = io.Copy(file, content)
	return
}

func (fc *fileCache) Delete(key Key) (err error) {
	// TODO: take lock per key for more efficiency vs the complexity of impl.
	fc.mux.Lock()
	defer fc.mux.Unlock()

	err = os.Remove(fc.path(key))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return
}
