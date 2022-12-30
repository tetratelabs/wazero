package platform

import (
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_StatTimes(t *testing.T) {
	tmpDir := t.TempDir()

	file := path.Join(tmpDir, "file")
	err := os.WriteFile(file, []byte{}, 0o700)
	require.NoError(t, err)

	type test struct {
		name                                     string
		atimeSec, atimeNsec, mtimeSec, mtimeNsec int64
	}
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{name: "positive", atimeSec: 123, atimeNsec: 4 * 1e3, mtimeSec: 567, mtimeNsec: 8 * 1e3},
		{name: "zero"},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{name: "negative", atimeSec: -123, atimeNsec: -4 * 1e3, mtimeSec: -567, mtimeNsec: -8 * 1e3},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := os.Chtimes(file, time.Unix(tc.atimeSec, tc.atimeNsec), time.Unix(tc.mtimeSec, tc.mtimeNsec))
			require.NoError(t, err)

			stat, err := os.Stat(file)
			require.NoError(t, err)

			atimeSec, atimeNsec, mtimeSec, mtimeNsec, _, _ := StatTimes(stat)
			if CompilerSupported() {
				require.Equal(t, atimeSec, tc.atimeSec)
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeSec, tc.mtimeSec)
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}
