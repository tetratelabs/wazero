package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/gojs"
	"github.com/tetratelabs/wazero/sys"
)

// main invokes Wasm compiled via `GOARCH=wasm GOOS=js`, which reports the star
// count of wazero.
//
// This shows how to integrate an HTTP client with wasm using gojs.
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		// WebAssembly 2.0 allows use of gojs.
		WithWasmCore2())
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Combine the above into our baseline config, overriding defaults.
	config := wazero.NewModuleConfig().
		// By default, I/O streams are discarded, so you won't see output.
		WithStdout(os.Stdout).WithStderr(os.Stderr)

	// Compile the WebAssembly module using the default configuration.
	compiled, err := r.CompileModule(ctx, compileWasm(), wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}

	// Instead of making real HTTP calls, return fake data.
	ctx = gojs.WithRoundTripper(ctx, &fakeGitHub{})

	// Execute the "run" function, which corresponds to "main" in cat/main.go.
	err = gojs.Run(ctx, r, compiled, config)
	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
		log.Panicln(err)
	} else if !ok {
		log.Panicln(err)
	}
}

// compile-time check to ensure fakeGitHub implements http.RoundTripper
var _ http.RoundTripper = &fakeGitHub{}

type fakeGitHub struct{}

func (f *fakeGitHub) RoundTrip(*http.Request) (*http.Response, error) {
	fakeResponse := `{"stargazers_count": 9999999}`
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        http.StatusText(http.StatusOK),
		Body:          io.NopCloser(strings.NewReader(fakeResponse)),
		ContentLength: int64(len(fakeResponse)),
	}, nil
}

// compileWasm compiles "stars/main.go" on demand as the binary generated is
// too big (>1MB) to check into the source tree.
func compileWasm() []byte {
	cmd := exec.Command("go", "build", "-o", "main.wasm", ".")

	_, thisFile, _, ok := runtime.Caller(1)
	if !ok {
		panic("couldn't read path to current file")
	}
	cmd.Dir = path.Join(path.Dir(thisFile), "stars")

	cmd.Env = append(os.Environ(), "GOARCH=wasm", "GOOS=js")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Panicf("go build: %v\n%s", err, out)
	}
	bin, err := os.ReadFile(path.Join(cmd.Dir, "main.wasm"))
	if err != nil {
		panic(err)
	}
	return bin
}
