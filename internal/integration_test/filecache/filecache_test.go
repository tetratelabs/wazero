package filecache

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest/v1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSpecTestCompilerCache(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	const cachePathKey = "FILE_CACHE_DIR"
	cacheDir := os.Getenv(cachePathKey)
	if len(cacheDir) == 0 {
		// This case, this is the parent of the test.
		dir := t.TempDir()

		// Get the executable path of this test.
		testExecutable, err := os.Executable()
		require.NoError(t, err)

		buf := bytes.NewBuffer(nil)

		// Execute this test multiple times with cachePathKey being the tmpDir
		var exp []string
		for i := 0; i < 5; i++ {
			cmd := exec.Command(testExecutable)
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", cachePathKey, dir))
			cmd.Stdout = buf
			cmd.Stderr = buf
			err = cmd.Run()
			require.NoError(t, err)
			exp = append(exp, "PASS\n")
		}

		// Ensures that the tests actually run 5 times.
		require.Equal(t, strings.Join(exp, ""), buf.String())
	} else {
		// Run the spectest with the file cache.
		ctx := experimental.WithCompilationCacheDirName(context.Background(), cacheDir)
		spectest.Run(t, v1.Testcases, ctx, compiler.NewEngine, v1.EnabledFeatures)

		// Check the number of caches as toehold.
		files, err := os.ReadDir(cacheDir)
		require.NoError(t, err)
		require.True(t, len(files) > 0)
	}
}
