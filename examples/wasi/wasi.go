package main

import (
	"embed"
	_ "embed"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/wasi"
	"io/fs"
	"log"
	"os"
)

// catFS is an embedded filesystem limited to test.txt
//go:embed testdata/test.txt
var catFS embed.FS

// catWasm was compiled the TinyGo source testdata/cat.go
//go:embed testdata/cat.wasm
var catWasm []byte

// main writes an input file to stdout, just like `cat`.
//
// This is a basic introduction to the WebAssembly System Interface (WASI).
// See https://github.com/WebAssembly/WASI
func main() {
	r := wazero.NewRuntime()

	// Since wazero uses fs.FS, we can use standard libraries to do things like trim the leading path.
	rooted, err := fs.Sub(catFS, "testdata")
	if err != nil {
		log.Fatal(err)
	}

	// Combine the above into our baseline config, overriding defaults (which discard stdout and have no file system).
	config := wazero.NewModuleConfig().WithStdout(os.Stdout).WithFS(rooted)

	// Instantiate WASI, which implements system I/O such as console output.
	wm, err := wasi.InstantiateSnapshotPreview1(r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close()

	// InstantiateModuleFromCodeWithConfig runs the "_start" function which is what TinyGo compiles "main" to.
	// * Set the program name (arg[0]) to "wasi" and add args to write "test.txt" to stdout twice.
	// * We use both "/test.txt" and "./test.txt" because WithFS by default maps the workdir "." to "/".
	cat, err := r.InstantiateModuleFromCodeWithConfig(catWasm, config.WithArgs("wasi", "/test.txt", "./test.txt"))
	if err != nil {
		log.Fatal(err)
	}
	defer cat.Close()
}
