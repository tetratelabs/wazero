package compilationcache

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
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
}

func (f *fileCache) path(key Key) string {
	return path.Join(f.dirPath, hex.EncodeToString(key[:]))
}

func (f *fileCache) Get(key Key) (content io.ReadCloser, ok bool, err error) {
	content, err = os.Open(f.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	} else {
		return content, true, nil
	}
}

func (f *fileCache) Add(key Key, content io.Reader) (err error) {
	file, err := os.Create(f.path(key))
	if err != nil {
		return
	}
	defer file.Close()
	_, err = io.Copy(file, content)
	return
}

func (f *fileCache) Delete(key Key) (err error) {
	err = os.Remove(f.path(key))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return
}
