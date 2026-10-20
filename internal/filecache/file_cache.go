package filecache

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// New returns a new Cache implemented by fileCache.
func New(dir string) Cache {
	return newFileCache(dir)
}

func newFileCache(dir string) *fileCache {
	return &fileCache{dirPath: dir}
}

// NewReadOnly requires cache hits and never modifies directory contents.
func NewReadOnly(dir string) Cache {
	return &fileCache{dirPath: dir, readOnly: true}
}

// ReadOnly reports whether c requires persistent cache hits.
func ReadOnly(c Cache) bool {
	r, ok := c.(interface{ ReadOnly() bool })
	return ok && r.ReadOnly()
}

func (fc *fileCache) ReadOnly() bool { return fc.readOnly }

// fileCache persists compiled functions into dirPath.
//
// Note: this can be expanded to do binary signing/verification, set TTL on each entry, etc.
type fileCache struct {
	dirPath  string
	readOnly bool
}

func (fc *fileCache) path(key Key) string {
	return path.Join(fc.dirPath, hex.EncodeToString(key[:]))
}

func (fc *fileCache) Get(key Key) (content io.ReadCloser, ok bool, err error) {
	f, err := os.Open(fc.path(key))
	if errors.Is(err, os.ErrNotExist) {
		if fc.readOnly {
			return nil, false, fmt.Errorf("%w: %s", ErrMiss, fc.path(key))
		}
		return nil, false, nil
	} else if err != nil {
		if fc.readOnly {
			return nil, false, fmt.Errorf("%w: %w", ErrIO, err)
		}
		return nil, false, err
	} else {
		return f, true, nil
	}
}

func (fc *fileCache) Add(key Key, content io.Reader) (err error) {
	if fc.readOnly {
		return ErrReadOnly
	}
	path := fc.path(key)
	dirPath, fileName := filepath.Split(path)

	file, err := os.CreateTemp(dirPath, fileName+".*.tmp")
	if err != nil {
		return
	}
	defer func() {
		file.Close()
		if err != nil {
			_ = os.Remove(file.Name())
		}
	}()
	if _, err = io.Copy(file, content); err != nil {
		return
	}
	if err = file.Sync(); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}
	err = os.Rename(file.Name(), path)
	return
}

func (fc *fileCache) Delete(key Key) (err error) {
	if fc.readOnly {
		return ErrReadOnly
	}
	err = os.Remove(fc.path(key))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return
}
