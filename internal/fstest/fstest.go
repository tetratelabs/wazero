// Package fstest defines filesystem test cases that help validate host
// functions implementing WASI and `GOARCH=wasm GOOS=js`. Tests are defined
// here to reduce duplication and drift.
//
// Here's an example using this inside code that compiles to wasm.
//
//	if err := fstest.TestFS(os.DirFS); err != nil {
//		log.Panicln(err)
//	}
//
// Failures found here should result in new tests in the appropriate package,
// for example, gojs, syscallfs or wasi_snapshot_preview1.
//
// This package must have no dependencies. Otherwise, compiling this with
// TinyGo or `GOARCH=wasm GOOS=js` can become bloated or complicated.
package fstest

import (
	"io/fs"
	"os"
	"path"
	"testing/fstest"
)

// TODO: Add file times and write them so that we can make stat tests.
var files = []struct {
	name string
	file *fstest.MapFile
}{
	{name: "empty.txt", file: &fstest.MapFile{Mode: 0o600}},
	{name: "animals.txt", file: &fstest.MapFile{Data: []byte(`bear
cat
shark
dinosaur
human
`), Mode: 0o644}},
	{name: "sub", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},
	{name: "sub/test.txt", file: &fstest.MapFile{Data: []byte("greet sub dir\n"), Mode: 0o444}},
	{name: "emptydir", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},
	{name: "dir", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},    // for readDir tests...
	{name: "dir/-", file: &fstest.MapFile{Mode: 0o400}},               // len = 24+1 = 25
	{name: "dir/a-", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}, // len = 24+2 = 26
	{name: "dir/ab-", file: &fstest.MapFile{Mode: 0o400}},             // len = 24+3 = 27
}

// FS includes all test files.
var FS = func() fs.ReadDirFS {
	testFS := make(fstest.MapFS, len(files))
	for _, nf := range files {
		testFS[nf.name] = nf.file
	}
	return testFS
}()

// WriteTestFiles writes files defined in FS to the given directory.
// This is used for implementations like os.DirFS.
func WriteTestFiles(tmpDir string) (err error) {
	// Don't use a map as the iteration order is inconsistent and can result in
	// files created prior to their directories.
	for _, nf := range files {
		if err = writeTestFile(tmpDir, nf.name, nf.file); err != nil {
			return
		}
	}
	return
}

// TestFS runs fstest.TestFS on the given input which is either FS or includes
// files written by WriteTestFiles.
func TestFS(testfs fs.FS) error {
	expected := make([]string, 0, len(files))
	for _, nf := range files {
		expected = append(expected, nf.name)
	}
	return fstest.TestFS(testfs, expected...)
}

func writeTestFile(tmpDir, name string, file *fstest.MapFile) error {
	fullPath := path.Join(tmpDir, name)
	if mode := file.Mode; mode&fs.ModeDir != 0 {
		return os.Mkdir(fullPath, mode)
	} else {
		return os.WriteFile(fullPath, file.Data, mode)
	}
}
