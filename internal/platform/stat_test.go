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
		name                 string
		atimeNsec, mtimeNsec int64
	}
	// Note: This sets microsecond granularity because Windows doesn't support
	// nanosecond.
	tests := []test{
		{
			name:      "positive",
			atimeNsec: time.Unix(123, 4*1e3).UnixNano(),
			mtimeNsec: time.Unix(567, 8*1e3).UnixNano(),
		},
		{name: "zero"},
	}

	// linux and freebsd report inaccurate results when the input ts is negative.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		tests = append(tests,
			test{
				name:      "negative",
				atimeNsec: time.Unix(-123, -4*1e3).UnixNano(),
				mtimeNsec: time.Unix(-567, -8*1e3).UnixNano(),
			},
		)
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := os.Chtimes(file, time.UnixMicro(tc.atimeNsec/1e3), time.UnixMicro(tc.mtimeNsec/1e3))
			require.NoError(t, err)

			stat, err := os.Stat(file)
			require.NoError(t, err)

			atimeNsec, mtimeNsec, _ := StatTimes(stat)
			if CompilerSupported() {
				require.Equal(t, atimeNsec, tc.atimeNsec)
			} // else only mtimes will return.
			require.Equal(t, mtimeNsec, tc.mtimeNsec)
		})
	}
}
