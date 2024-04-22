// Package fstest defines filesystem test cases that help validate host
// functions implementing WASI. Tests are defined
// here to reduce duplication and drift.
//
// Here's an example using this inside code that compiles to wasm.
//
//	if err := fstest.WriteTestFiles(tmpDir); err != nil {
//		log.Panicln(err)
//	}
//	if err := fstest.TestFS(os.DirFS(tmpDir)); err != nil {
//		log.Panicln(err)
//	}
//
// Failures found here should result in new tests in the appropriate package,
// for example, sysfs or wasi_snapshot_preview1.
package fstest

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing/fstest"
	"time"
)

var files = []struct {
	name string
	file *fstest.MapFile
}{
	{
		name: ".", // is defined only for the sake of assigning mtim.
		file: &fstest.MapFile{
			Mode:    fs.ModeDir | 0o755,
			ModTime: time.Unix(1609459200, 0),
		},
	},
	{name: "empty.txt", file: &fstest.MapFile{Mode: 0o600}},
	{name: "emptydir", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},
	{name: "animals.txt", file: &fstest.MapFile{Data: []byte(`bear
cat
shark
dinosaur
human
`), Mode: 0o644, ModTime: time.Unix(1667482413, 0)}},
	{name: "sub", file: &fstest.MapFile{
		Mode:    fs.ModeDir | 0o755,
		ModTime: time.Unix(1640995200, 0),
	}},
	{name: "sub/test.txt", file: &fstest.MapFile{
		Data:    []byte("greet sub dir\n"),
		Mode:    0o444,
		ModTime: time.Unix(1672531200, 0),
	}},
	{name: "dir", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}},    // for readDir tests...
	{name: "dir/-", file: &fstest.MapFile{Mode: 0o400}},               // len = 24+1 = 25
	{name: "dir/a-", file: &fstest.MapFile{Mode: fs.ModeDir | 0o755}}, // len = 24+2 = 26
	{name: "dir/ab-", file: &fstest.MapFile{Mode: 0o400}},             // len = 24+3 = 27
}

// FS includes all test files.
var FS = func() fstest.MapFS {
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

	// The below is similar to code in os_test.go. In summary, Windows mtime
	// can be inconsistent between DirEntry.Info and File.Stat. The latter is
	// authoritative (via GetFileInformationByHandle), but the former can be
	// out of sync (via FindFirstFile). Explicitly calling os.Chtimes syncs
	// these. See golang.org/issues/42637.
	if runtime.GOOS == "windows" {
		return filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}

			// os.Stat uses GetFileInformationByHandle internally.
			st, err := os.Stat(path)
			if err != nil {
				return err
			} else if st.ModTime() == info.ModTime() {
				return nil // synced!
			}

			// Otherwise, we need to sync the timestamps.
			atimeNsec, mtimeNsec := timesFromFileInfo(st)
			return os.Chtimes(path, time.Unix(0, atimeNsec), time.Unix(0, mtimeNsec))
		})
	}
	return
}

// TestFS runs fstest.TestFS on the given input which is either FS or includes
// files written by WriteTestFiles.
func TestFS(testfs fs.FS) error {
	expected := make([]string, 0, len(files))
	for _, nf := range files[1:] { // skip "."
		expected = append(expected, nf.name)
	}
	return fstest.TestFS(testfs, expected...)
}

var defaultTime = time.Unix(1577836800, 0)

func writeTestFile(tmpDir, name string, file *fstest.MapFile) (err error) {
	fullPath := path.Join(tmpDir, name)
	if mode := file.Mode; mode&fs.ModeDir != 0 {
		if name != "." {
			err = os.Mkdir(fullPath, mode)
		}
	} else {
		err = os.WriteFile(fullPath, file.Data, mode)
	}

	if err != nil {
		return
	}

	mtim := file.ModTime
	if mtim.Unix() == 0 {
		mtim = defaultTime
	}
	return os.Chtimes(fullPath, mtim, mtim)
}
