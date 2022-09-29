package main

import (
	"io/fs"
	"strings"
)

type compositeFS struct {
	paths map[string]fs.FS
}

func (c *compositeFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	for path, f := range c.paths {
		if !strings.HasPrefix(name, path) {
			continue
		}
		rest := name[len(path):]
		if len(rest) == 0 {
			// Special case reading directory
			rest = "."
		} else {
			// fs.Open requires a relative path
			if rest[0] == '/' {
				rest = rest[1:]
			}
		}
		file, err := f.Open(rest)
		if err == nil {
			return file, err
		}
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
