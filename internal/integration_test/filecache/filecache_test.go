package filecache

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	v1 "github.com/tetratelabs/wazero/internal/integration_test/spectest/v1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestFileCache_compiler(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}
	runAllFileCacheTests(t, wazero.NewRuntimeConfigCompiler())
}

func runAllFileCacheTests(t *testing.T, config wazero.RuntimeConfig) {
	t.Run("spectest", func(t *testing.T) {
		testSpecTestCompilerCache(t, config)
	})
	t.Run("listeners", func(t *testing.T) {
		testListeners(t, config)
	})
	t.Run("close on context done", func(t *testing.T) {
		testWithCloseOnContextDone(t, config)
	})
}

func testSpecTestCompilerCache(t *testing.T, config wazero.RuntimeConfig) {
	const cachePathKey = "FILE_CACHE_DIR"
	cacheDir := os.Getenv(cachePathKey)
	if len(cacheDir) == 0 {
		// This case, this is the parent of the test.
		cacheDir = t.TempDir()

		// Before running test, no file should exist in the directory.
		files, err := os.ReadDir(cacheDir)
		require.NoError(t, err)
		require.True(t, len(files) == 0)

		// Get the executable path of this test.
		testExecutable, err := os.Executable()
		require.NoError(t, err)

		// Execute this test multiple times with the env $cachePathKey=cacheDir, so that
		// the subsequent execution of this test will enter the following "else" block.
		var exp []string
		buf := bytes.NewBuffer(nil)
		for i := 0; i < 2; i++ {
			cmd := exec.Command(testExecutable)
			cmd.Args = append(cmd.Args, fmt.Sprintf("-test.run=%s", t.Name()))
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", cachePathKey, cacheDir))
			cmd.Stdout = buf
			cmd.Stderr = buf
			err = cmd.Run()
			require.NoError(t, err, buf.String())
			exp = append(exp, "PASS\n")
		}

		// Ensures that the tests actually run 2 times.
		require.Equal(t, strings.Join(exp, ""), buf.String())

		// Check the number of cache files is greater than zero.
		files, err = os.ReadDir(cacheDir)
		require.NoError(t, err)
		require.True(t, len(files) > 0)
	} else {
		// Run the spectest with the file cache.
		cc, err := wazero.NewCompilationCacheWithDir(cacheDir)
		require.NoError(t, err)
		spectest.Run(t, v1.Testcases, context.Background(),
			config.WithCompilationCache(cc).WithCoreFeatures(api.CoreFeaturesV1))
	}
}

// TestListeners ensures that compilation cache works as expected on and off with respect to listeners.
func testListeners(t *testing.T, config wazero.RuntimeConfig) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	var (
		zero    uint32 = 0
		wasmBin        = binaryencoding.EncodeModule(&wasm.Module{
			TypeSection:     []wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0, 0, 0, 0},
			CodeSection: []wasm.Code{
				{Body: []byte{wasm.OpcodeCall, 1, wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeCall, 2, wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeCall, 3, wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeEnd}},
			},
			StartSection: &zero,
			NameSection: &wasm.NameSection{
				FunctionNames: wasm.NameMap{{Index: 0, Name: "1"}, {Index: 1, Name: "2"}, {Index: 2, Name: "3"}, {Index: 3, Name: "4"}},
				ModuleName:    "test",
			},
		})
	)

	t.Run("always on", func(t *testing.T) {
		dir := t.TempDir()

		out := bytes.NewBuffer(nil)
		ctxWithListener := experimental.WithFunctionListenerFactory(
			context.Background(), logging.NewLoggingListenerFactory(out))

		{
			cc, err := wazero.NewCompilationCacheWithDir(dir)
			require.NoError(t, err)
			rc := config.WithCompilationCache(cc)

			r := wazero.NewRuntimeWithConfig(ctxWithListener, rc)
			_, err = r.CompileModule(ctxWithListener, wasmBin)
			require.NoError(t, err)
			err = r.Close(ctxWithListener)
			require.NoError(t, err)
		}

		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc)
		r := wazero.NewRuntimeWithConfig(ctxWithListener, rc)
		_, err = r.Instantiate(ctxWithListener, wasmBin)
		require.NoError(t, err)
		err = r.Close(ctxWithListener)
		require.NoError(t, err)

		// Ensures that compilation cache works with listeners.
		require.Equal(t, `--> test.1()
	--> test.2()
		--> test.3()
			--> test.4()
			<--
		<--
	<--
<--
`, out.String())
	})

	t.Run("with->without", func(t *testing.T) {
		dir := t.TempDir()

		// Compile with listeners.
		{
			cc, err := wazero.NewCompilationCacheWithDir(dir)
			require.NoError(t, err)
			rc := config.WithCompilationCache(cc)

			out := bytes.NewBuffer(nil)
			ctxWithListener := experimental.WithFunctionListenerFactory(
				context.Background(), logging.NewLoggingListenerFactory(out))
			r := wazero.NewRuntimeWithConfig(ctxWithListener, rc)
			_, err = r.CompileModule(ctxWithListener, wasmBin)
			require.NoError(t, err)
			err = r.Close(ctxWithListener)
			require.NoError(t, err)
		}

		// Then compile without listeners -> run it.
		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc)
		r := wazero.NewRuntimeWithConfig(context.Background(), rc)
		_, err = r.Instantiate(context.Background(), wasmBin)
		require.NoError(t, err)
		err = r.Close(context.Background())
		require.NoError(t, err)
	})

	t.Run("without->with", func(t *testing.T) {
		dir := t.TempDir()

		// Compile without listeners.
		{
			cc, err := wazero.NewCompilationCacheWithDir(dir)
			require.NoError(t, err)
			rc := config.WithCompilationCache(cc)
			r := wazero.NewRuntimeWithConfig(context.Background(), rc)
			_, err = r.CompileModule(context.Background(), wasmBin)
			require.NoError(t, err)
			err = r.Close(context.Background())
			require.NoError(t, err)
		}

		// Then compile with listeners -> run it.
		out := bytes.NewBuffer(nil)
		ctxWithListener := experimental.WithFunctionListenerFactory(
			context.Background(), logging.NewLoggingListenerFactory(out))

		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc)
		r := wazero.NewRuntimeWithConfig(ctxWithListener, rc)
		_, err = r.Instantiate(ctxWithListener, wasmBin)
		require.NoError(t, err)
		err = r.Close(ctxWithListener)
		require.NoError(t, err)

		// Ensures that compilation cache works with listeners.
		require.Equal(t, `--> test.1()
	--> test.2()
		--> test.3()
			--> test.4()
			<--
		<--
	<--
<--
`, out.String())
	})
}

// TestWithCloseOnContextDone ensures that compilation cache works as expected on and off with respect to WithCloseOnContextDone config.
func testWithCloseOnContextDone(t *testing.T, config wazero.RuntimeConfig) {
	var (
		zero    uint32 = 0
		wasmBin        = binaryencoding.EncodeModule(&wasm.Module{
			TypeSection:     []wasm.FunctionType{{}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeLoop, 0,
					wasm.OpcodeBr, 0,
					wasm.OpcodeEnd,
					wasm.OpcodeEnd,
				}},
			},
			StartSection: &zero,
		})
	)

	t.Run("always on", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()
		{
			cc, err := wazero.NewCompilationCacheWithDir(dir)
			require.NoError(t, err)
			rc := config.WithCompilationCache(cc).WithCloseOnContextDone(true)

			r := wazero.NewRuntimeWithConfig(ctx, rc)
			_, err = r.CompileModule(ctx, wasmBin)
			require.NoError(t, err)
			err = r.Close(ctx)
			require.NoError(t, err)
		}

		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc).WithCloseOnContextDone(true)
		r := wazero.NewRuntimeWithConfig(ctx, rc)

		timeoutCtx, done := context.WithTimeout(ctx, time.Second)
		defer done()
		_, err = r.Instantiate(timeoutCtx, wasmBin)
		require.EqualError(t, err, "module closed with context deadline exceeded")
		err = r.Close(ctx)
		require.NoError(t, err)
	})

	t.Run("off->on", func(t *testing.T) {
		dir := t.TempDir()
		ctx := context.Background()
		{
			cc, err := wazero.NewCompilationCacheWithDir(dir)
			require.NoError(t, err)
			rc := config.WithCompilationCache(cc).WithCloseOnContextDone(false)

			r := wazero.NewRuntimeWithConfig(ctx, rc)
			_, err = r.CompileModule(ctx, wasmBin)
			require.NoError(t, err)
			err = r.Close(ctx)
			require.NoError(t, err)
		}

		cc, err := wazero.NewCompilationCacheWithDir(dir)
		require.NoError(t, err)
		rc := config.WithCompilationCache(cc).WithCloseOnContextDone(true)
		r := wazero.NewRuntimeWithConfig(ctx, rc)

		timeoutCtx, done := context.WithTimeout(ctx, time.Second)
		defer done()
		_, err = r.Instantiate(timeoutCtx, wasmBin)
		require.EqualError(t, err, "module closed with context deadline exceeded")
		err = r.Close(ctx)
		require.NoError(t, err)
	})
}
