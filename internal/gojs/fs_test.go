package gojs_test

import (
	_ "embed"
	"io/fs"
	"log"
	"os"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/fs/main.go
var fsGo string

var testFS = fstest.MapFS{
	"empty.txt":    {},
	"test.txt":     {Data: []byte("animals")},
	"sub":          {Mode: fs.ModeDir},
	"sub/test.txt": {Data: []byte("greet sub dir\n")},
}

// TestMain ensures fstest works normally
func TestMain(m *testing.M) {
	if d, err := fs.Sub(testFS, "sub"); err != nil {
		log.Fatalln(err)
	} else if err = fstest.TestFS(d, "test.txt"); err != nil {
		log.Fatalln(err)
	}
	os.Exit(m.Run())
}

func Test_fs(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, fsGo, wazero.NewModuleConfig().
		WithFS(testFS))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `TestFS ok
wd ok
Not a directory
/test.txt ok
test.txt ok
contents: animals
empty: 
`, stdout)
}
