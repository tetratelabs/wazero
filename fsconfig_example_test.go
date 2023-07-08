package wazero_test

import (
	"embed"
	"io/fs"
	"log"
	"math"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
)

//go:embed testdata/index.html
var testdataIndex embed.FS

var moduleConfig wazero.ModuleConfig

// This example shows how to configure an embed.FS.
func Example_withFSConfig_embedFS() {
	// Strip the embedded path testdata/
	rooted, err := fs.Sub(testdataIndex, "testdata")
	if err != nil {
		log.Panicln(err)
	}

	moduleConfig = wazero.NewModuleConfig().
		// Make "index.html" accessible to the guest as "/index.html".
		WithFSConfig(wazero.NewFSConfig().WithFSMount(rooted, "/"))
}

type sysStatFS struct{ fs.FS }

func (f *sysStatFS) Open(name string) (fs.File, error) {
	if f, err := f.FS.Open(name); err != nil {
		return nil, err
	} else {
		return &sysStatFile{f}, nil
	}
}

// sysStatFile only intercepts fs.File to keep the example simple.
// wazero supports more interfaces like fs.ReadDirFile and io.Seeker.
type sysStatFile struct{ fs.File }

func (f *sysStatFile) Stat() (fs.FileInfo, error) {
	if info, err := f.File.Stat(); err != nil {
		return nil, err
	} else {
		st := sys.NewStat_t(info)
		st.Ino = math.MaxUint64 // arbitrary non-zero value
		return &sysStatFileInfo{info, st}, nil
	}
}

type sysStatFileInfo struct {
	fs.FileInfo
	st sys.Stat_t
}

func (f *sysStatFileInfo) Sys() any { return &f.st }

var testFS fs.FS

// This shows how you can add inode data to files served by a fs.FS.
func Example_withFSConfig_fsWithIno() {
	// Wrap the filesystem with one that sets FileInfo.Stat.Sys to *sys.Stat_t
	withIno := &sysStatFS{testFS}

	moduleConfig = wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithFSMount(withIno, "/"))
}
