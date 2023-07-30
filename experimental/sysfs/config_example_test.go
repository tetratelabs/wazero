package sysfs_test

import (
	"io/fs"
	"testing/fstest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/sysfs"
)

var moduleConfig wazero.ModuleConfig

// This example shows how to configure a sysfs.NewDirFS
func ExampleNewDirFS() {
	root := sysfs.NewDirFS(".")

	moduleConfig = wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().(sysfs.FSConfig).WithSysFSMount(root, "/"))
}

// This example shows how to configure a sysfs.NewReadFS
func ExampleNewReadFS() {
	root := sysfs.NewDirFS(".")
	readOnly := sysfs.NewReadFS(root)

	moduleConfig = wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().(sysfs.FSConfig).WithSysFSMount(readOnly, "/"))
}

// This example shows how to adapt a fs.FS as a sys.FS
func ExampleAdapt() {
	m := fstest.MapFS{
		"a/b.txt": &fstest.MapFile{Mode: 0o666},
		".":       &fstest.MapFile{Mode: 0o777 | fs.ModeDir},
	}
	root := sysfs.Adapt(m)

	moduleConfig = wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().(sysfs.FSConfig).WithSysFSMount(root, "/"))
}
