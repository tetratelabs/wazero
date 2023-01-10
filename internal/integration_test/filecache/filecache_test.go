package filecache

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	v1 "github.com/tetratelabs/wazero/internal/integration_test/spectest/v1"
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
		fc := filecache.New(cacheDir)
		spectest.Run(t, v1.Testcases, context.Background(), fc, compiler.NewEngine, v1.EnabledFeatures)
	}
}
