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
	"github.com/tetratelabs/wazero/experimental"
	gojs "github.com/tetratelabs/wazero/imports/go"
)

func compileAndRun(ctx context.Context, arg string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	ns := rt.NewNamespace(ctx)
	builder := rt.NewHostModuleBuilder("go")
	gojs.NewFunctionExporter().ExportFunctions(builder)
	if _, err = builder.Instantiate(ctx, ns); err != nil {
		return
	}

	// Note: this hits the file cache.
	compiled, err := rt.CompileModule(testCtx, testBin)
	if err != nil {
		log.Panicln(err)
	}

	err = gojs.Run(ctx, ns, compiled, config.WithStdout(&stdoutBuf).WithStderr(&stderrBuf).
		WithArgs("test", arg))
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

// testBin is not checked in as it is >7.5MB
var testBin []byte

// testCtx is configured in TestMain to re-use wazero's compilation cache.
var (
	testCtx context.Context
	testFS  = fstest.MapFS{
		"empty.txt":    {},
		"test.txt":     {Data: []byte("animals\n")},
		"sub":          {Mode: fs.ModeDir},
		"sub/test.txt": {Data: []byte("greet sub dir\n")},
	}
	rt wazero.Runtime
)

func TestMain(m *testing.M) {
	// For some reason, windows and freebsd fail to compile with exit status 1.
	if o := runtime.GOOS; o != "darwin" && o != "linux" {
		log.Println("gojs: skipping due to not yet supported OS:", o)
		os.Exit(0)
	}

	// Find the go binary (if present), and compile the Wasm binary.
	goBin, err := findGoBin()
	if err != nil {
		log.Println("gojs: skipping due missing Go binary:", err)
		os.Exit(0)
	}
	if err = compileJsWasm(goBin); err != nil {
		log.Panicln(err)
	}

	// Define a compilation cache so that tests run faster. This works because
	// all tests use the same binary.
	compilationCacheDir, err := os.MkdirTemp("", "gojs")
	if err != nil {
		log.Panicln(err)
	}
	defer os.RemoveAll(compilationCacheDir)
	testCtx, err = experimental.WithCompilationCacheDirName(context.Background(), compilationCacheDir)
	if err != nil {
		log.Panicln(err)
	}

	// Seed wazero's compilation cache to see any error up-front and to prevent
	// one test from a cache-miss performance penalty.
	rt = wazero.NewRuntimeWithConfig(testCtx, wazero.NewRuntimeConfig())
	_, err = rt.CompileModule(testCtx, testBin)
	if err != nil {
		log.Panicln(err)
	}

	var exit int
	defer func() {
		rt.Close(testCtx)
		os.Exit(exit)
	}()

	// Configure fs test data
	if d, err := fs.Sub(testFS, "sub"); err != nil {
		log.Panicln(err)
	} else if err = fstest.TestFS(d, "test.txt"); err != nil {
		log.Panicln(err)
	}
	exit = m.Run()
}

// compileJsWasm allows us to generate a binary with runtime.GOOS=js and
// runtime.GOARCH=wasm. This intentionally does so on-demand, as it allows us
// to test the user's current version of Go, as opposed to a specific one.
// For example, this allows testing both Go 1.18 and 1.19 in CI.
func compileJsWasm(goBin string) error {
	// Prepare the working directory.
	workDir, err := os.MkdirTemp("", "example")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bin := path.Join(workDir, "out.wasm")
	cmd := exec.CommandContext(ctx, goBin, "build", "-o", bin, ".") //nolint:gosec
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "GOWASM=satconv,signext")
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
