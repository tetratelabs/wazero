package gojs_test

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	gojs "github.com/tetratelabs/wazero/imports/go"
	"github.com/tetratelabs/wazero/internal/fstest"
	internalgojs "github.com/tetratelabs/wazero/internal/gojs"
	"github.com/tetratelabs/wazero/internal/gojs/run"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
)

func compileAndRun(ctx context.Context, arg string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		// https://github.com/tetratelabs/wazero/issues/992
		WithMemoryCapacityFromMax(true).
		WithCompilationCache(cache))
	return compileAndRunWithRuntime(ctx, rt, arg, config) // use global runtime
}

func compileAndRunWithRuntime(ctx context.Context, r wazero.Runtime, arg string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	builder := r.NewHostModuleBuilder("go")
	gojs.NewFunctionExporter().ExportFunctions(builder)
	if _, err = builder.Instantiate(ctx); err != nil {
		return
	}

	// Note: this hits the file cache.
	compiled, err := r.CompileModule(testCtx, testBin)
	if err != nil {
		log.Panicln(err)
	}

	var s *internalgojs.State
	s, err = run.RunAndReturnState(ctx, r, compiled, config.
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithArgs("test", arg))
	if err == nil {
		if !reflect.DeepEqual(s, internalgojs.NewState(ctx)) {
			log.Panicf("unexpected state: %v\n", s)
		}
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

// testBin is not checked in as it is >7.5MB
var testBin []byte

// testCtx is configured in TestMain to re-use wazero's compilation cache.
var (
	testCtx = context.Background()
	testFS  = fstest.FS
	cache   = wazero.NewCompilationCache()
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
	cache, err := wazero.NewCompilationCacheWithDir(compilationCacheDir)
	if err != nil {
		log.Panicln(err)
	}

	// In order to avoid race condition on scheduleTimeoutEvent, we need to set the memory max
	// and WithMemoryCapacityFromMax(true) above.
	// https://github.com/tetratelabs/wazero/issues/992
	//
	// TODO: Maybe add WithMemoryMax API?
	parsed, err := binaryformat.DecodeModule(testBin, api.CoreFeaturesV2, wasm.MemoryLimitPages, false, false, false)
	if err != nil {
		log.Panicln(err)
	}
	parsed.MemorySection.Max = 1000
	parsed.MemorySection.IsMaxEncoded = true
	testBin = binaryformat.EncodeModule(parsed)

	// Seed wazero's compilation cache to see any error up-front and to prevent
	// one test from a cache-miss performance penalty.
	r := wazero.NewRuntimeWithConfig(testCtx, wazero.NewRuntimeConfig().WithCompilationCache(cache))
	_, err = r.CompileModule(testCtx, testBin)
	if err != nil {
		log.Panicln(err)
	}

	var exit int
	defer func() {
		cache.Close(testCtx)
		r.Close(testCtx)
		os.Exit(exit)
	}()
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
