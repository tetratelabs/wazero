package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

type compositeFS struct {
	paths map[string]fs.FS
}

func (c *compositeFS) Open(name string) (fs.File, error) {
	name = filepath.Clean(name)
	for path, f := range c.paths {
		if !strings.HasPrefix(name, path) {
			continue
		}
		rest := name[len(path):]
		if len(rest) == 0 {
			continue
		}
		if rest[0] == '/' {
			rest = rest[1:]
		}
		file, err := f.Open(rest)
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			return file, err
		}
	}

	return nil, fs.ErrNotExist
}
