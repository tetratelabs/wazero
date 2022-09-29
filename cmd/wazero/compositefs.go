package main

import (
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
		// TODO(anuraaga): Currently leading slashes are always removed when calling fs.Open so we don't know
		// if the original open request is relative or absolute here. For now, treat a mount path of "." the same
		// as the root.
		if path == "." {
			path = ""
		}
		if !strings.HasPrefix(name, path) && path != "." {
			continue
		}
		rest := name[len(path):]
		if len(rest) == 0 {
			continue
		}
		if rest[0] == '/' {
			rest = rest[1:]
		}
		// Starting with .. or . are not valid for fs.FS. We may have actually
		// mounted guest paths starting with those, and will find it in c.paths
		// if that is the case.
		if !fs.ValidPath(rest) {
			continue
		}
		file, err := f.Open(rest)
		if err == nil {
			return file, err
		}
	}

	return nil, fs.ErrNotExist
}
