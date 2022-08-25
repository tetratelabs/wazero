package gojs_test

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/gojs"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func compileAndRun(ctx context.Context, arg string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	r := wazero.NewRuntimeWithConfig(testCtx, wazero.NewRuntimeConfig().WithWasmCore2())
	defer r.Close(ctx)

	compiled, compileErr := r.CompileModule(ctx, testBin, wazero.NewCompileConfig())
	if compileErr != nil {
		err = compileErr
		return
	}

	err = gojs.Run(ctx, r, compiled, config.WithStdout(&stdoutBuf).WithStderr(&stderrBuf).
		WithArgs("test", arg))
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

var testBin []byte

var testFS = fstest.MapFS{
	"empty.txt":    {},
	"test.txt":     {Data: []byte("animals")},
	"sub":          {Mode: fs.ModeDir},
	"sub/test.txt": {Data: []byte("greet sub dir\n")},
}

func TestMain(m *testing.M) {
	goBin, err := findGoBin()
	if err != nil {
		log.Println("skipping due missing Go binary:", err)
		os.Exit(0)
	}
	if err = compileJsWasm(goBin); err != nil {
		log.Panicln(err)
	}

	if d, err := fs.Sub(testFS, "sub"); err != nil {
		log.Fatalln(err)
	} else if err = fstest.TestFS(d, "test.txt"); err != nil {
		log.Fatalln(err)
	}
	os.Exit(m.Run())
}

// compileJsWasm allows us to generate a binary with runtime.GOOS=js and
// runtime.GOARCH=wasm. This intentionally does so on-demand, as it allows us
// to test the user's current version of Go, as opposed to a specific one.
// For example, this allows testing both Go 1.18 and 1.19 in CI.
func compileJsWasm(goBin string) error {
	// For some reason, windows and freebsd fail to compile with exit status 1.
	if os := runtime.GOOS; os != "darwin" && os != "linux" {
		return fmt.Errorf("not yet supported OS: %s", os)
	}

	// Prepare the working directory.
	workDir, err := os.MkdirTemp("", "example")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bin := path.Join(workDir, "out.wasm")
	cmd := exec.CommandContext(ctx, goBin, "build", "-o", bin, ".") //nolint:gosec
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Dir = "testdata"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't compile %s: %w", string(out), err)
	}

	testBin, err = os.ReadFile(bin) //nolint:gosec
	return err
}

func findGoBin() (string, error) {
	binName := "go"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", binName)
	if _, err := os.Stat(goBin); err == nil {
		return goBin, nil
	}
	// Now, search the path
	return exec.LookPath(binName)
}
