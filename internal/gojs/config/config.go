// Package config exists to avoid dependency cycles when keeping most of gojs
// code internal.
package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/tetratelabs/wazero/internal/platform"
)

type Config struct {
	OsWorkdir bool
	OsUser    bool

	Uid, Gid, Euid int
	Groups         []int

	// Workdir is the actual working directory value.
	Workdir string
	Umask   uint32
	Rt      http.RoundTripper
}

func NewConfig() *Config {
	return &Config{
		OsWorkdir: false,
		OsUser:    false,
		Uid:       0,
		Gid:       0,
		Euid:      0,
		Groups:    []int{0},
		Workdir:   "/",
		Umask:     uint32(0o0022),
		Rt:        nil,
	}
}

func (c *Config) Clone() *Config {
	ret := *c // copy except maps which share a ref
	return &ret
}

func (c *Config) Init() error {
	if c.OsWorkdir {
		workdir, err := os.Getwd()
		if err != nil {
			return err
		}
		// Ensure if used on windows, the input path is translated to a POSIX one.
		workdir = platform.ToPosixPath(workdir)
		// Strip the volume of the path, for example C:\
		c.Workdir = workdir[len(filepath.VolumeName(workdir)):]
	}

	// Windows does not support any of these properties
	if c.OsUser && runtime.GOOS != "windows" {
		c.Uid = syscall.Getuid()
		c.Gid = syscall.Getgid()
		c.Euid = syscall.Geteuid()
		var err error
		if c.Groups, err = syscall.Getgroups(); err != nil {
			return fmt.Errorf("couldn't read groups: %w", err)
		}
	}
	return nil
}
