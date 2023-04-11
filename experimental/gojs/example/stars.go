package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/gojs"
	"github.com/tetratelabs/wazero/sys"
)

// main invokes Wasm compiled via `GOARCH=wasm GOOS=js`, which reports the star
// count of wazero.
//
// This shows how to integrate an HTTP client with wasm using gojs.
func main() {
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Add the host functions used by `GOARCH=wasm GOOS=js`
	start := time.Now()
	gojs.MustInstantiate(ctx, r)

	goJsInstantiate := time.Since(start).Milliseconds()
	log.Printf("gojs.MustInstantiate took %dms", goJsInstantiate)

	// Combine the above into our baseline config, overriding defaults.
	moduleConfig := wazero.NewModuleConfig().
		// By default, I/O streams are discarded, so you won't see output.
		WithStdout(os.Stdout).WithStderr(os.Stderr)

	bin, err := os.ReadFile(path.Join("stars", "main.wasm"))
	if err != nil {
		log.Panicln(err)
	}

	// Compile the WebAssembly module using the default configuration.
	start = time.Now()
	compiled, err := r.CompileModule(ctx, bin)
	if err != nil {
		log.Panicln(err)
	}
	compilationTime := time.Since(start).Milliseconds()
	log.Printf("CompileModule took %dms", compilationTime)

	// Instead of making real HTTP calls, return fake data.
	config := gojs.NewConfig(moduleConfig).WithRoundTripper(&fakeGitHub{})

	// Execute the "run" function, which corresponds to "main" in stars/main.go.
	start = time.Now()
	err = gojs.Run(ctx, r, compiled, config)
	runTime := time.Since(start).Milliseconds()
	log.Printf("gojs.Run took %dms", runTime)
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
