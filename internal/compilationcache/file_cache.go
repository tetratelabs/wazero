package compilationcache

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
)

// FileCachePathKey is a context.Context Value key. Its value is a string
// representing the compilation cache directory.
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

// fileCache persists compiled functions into dirPath.
//
// Note: this can be expanded to do binary signing/verification, set TTL on each entry, etc.
type fileCache struct {
	dirPath string
	dirOk   bool
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
	fc.mux.RLock()
	unlock := fc.mux.RUnlock
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	f, err := os.Open(fc.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	} else {
		// Unlock is done inside the content.Close() at the call site.
		unlock = nil
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

	if err = fc.requireDir(); err != nil {
		return err
	}

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

// requireDir ensures the configured directory exists, notably before adding
// entries to it. This should be called under lock during Add.
func (fc *fileCache) requireDir() error {
	if fc.dirOk {
		return nil
	}
	if s, err := os.Stat(fc.dirPath); errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(fc.dirPath, 0o700); err != nil {
			return fmt.Errorf("fileCache: couldn't create dir %s: %w", fc.dirPath, err)
		}
	} else if err != nil {
		return fmt.Errorf("fileCache: couldn't open dir %s: %w", fc.dirPath, err)
	} else if !s.IsDir() {
		return fmt.Errorf("fileCache: expected dir at %s", fc.dirPath)
	}
	fc.dirOk = true
	return nil
}
