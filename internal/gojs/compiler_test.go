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
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/gojs"
	"github.com/tetratelabs/wazero/internal/fstest"
	internalgojs "github.com/tetratelabs/wazero/internal/gojs"
	"github.com/tetratelabs/wazero/internal/gojs/config"
	"github.com/tetratelabs/wazero/internal/gojs/run"
)

type newConfig func(moduleConfig wazero.ModuleConfig) (wazero.ModuleConfig, *config.Config)

func defaultConfig(moduleConfig wazero.ModuleConfig) (wazero.ModuleConfig, *config.Config) {
	return moduleConfig, config.NewConfig()
}

func compileAndRun(ctx context.Context, arg string, config newConfig) (stdout, stderr string, err error) {
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		// In order to avoid race condition on scheduleTimeoutEvent, we need to set the memory max
		// and WithMemoryCapacityFromMax(true) above. See #992.
		WithMemoryCapacityFromMax(true).
		// Set max to a high value, e.g. so that Test_stdio_large can pass.
		WithMemoryLimitPages(1024). // 64MB
		WithCompilationCache(cache))
	return compileAndRunWithRuntime(ctx, rt, arg, config) // use global runtime
}

func compileAndRunWithRuntime(ctx context.Context, r wazero.Runtime, arg string, config newConfig) (stdout, stderr string, err error) {
	// Note: this hits the file cache.
	var guest wazero.CompiledModule
	if guest, err = r.CompileModule(testCtx, testBin); err != nil {
		log.Panicln(err)
	}

	if _, err = gojs.Instantiate(ctx, r, guest); err != nil {
		return
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	mc, c := config(wazero.NewModuleConfig().
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithArgs("test", arg))

	ctx = experimental.WithCloseNotifier(ctx, experimental.CloseNotifyFunc(func(ctx context.Context, exitCode uint32) {
		s := ctx.Value(internalgojs.StateKey{})
		if want, have := internalgojs.NewState(c), s; !reflect.DeepEqual(want, have) {
			log.Panicf("unexpected state: want %#v, have %#v", want, have)
		}
	}))
	err = run.Run(ctx, r, guest, mc, c)
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
// For example, this allows testing both Go 1.19 and 1.20 in CI.
func compileJsWasm(goBin string) error {
	// Prepare the working directory.
	workdir, err := os.MkdirTemp("", "example")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workdir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bin := path.Join(workdir, "out.wasm")
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

// logString handles the "go" -> "gojs" module rename in Go 1.21
func logString(log bytes.Buffer) string {
	return strings.ReplaceAll(log.String(), "==> gojs", "==> go")
}
