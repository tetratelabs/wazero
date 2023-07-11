package sysfs

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSelect(t *testing.T) {
	t.Run("should return immediately with no fds and duration 0", func(t *testing.T) {
		for {
			dur := time.Duration(0)
			n, err := _select(0, nil, nil, nil, &dur)
			if err == sys.EINTR {
				t.Logf("Select interrupted")
				continue
			}
			require.NoError(t, err)
			require.Equal(t, 0, n)
			break
		}
	})

	t.Run("should wait for the given duration", func(t *testing.T) {
		dur := 250 * time.Millisecond
		var took time.Duration
		for {
			// On some platforms (e.g. Linux), the passed-in timeval is
			// updated by select(2). We are not accounting for this
			// in our implementation.
			start := time.Now()
			n, err := _select(0, nil, nil, nil, &dur)
			took = time.Since(start)
			if err == sys.EINTR {
				t.Logf("Select interrupted after %v", took)
				continue
			}
			require.NoError(t, err)
			require.Equal(t, 0, n)
			break
		}

		// On some platforms the actual timeout might be arbitrarily
		// less than requested.
		if took < dur {
			if runtime.GOOS == "linux" {
				// Linux promises to only return early if a file descriptor
				// becomes ready (not applicable here), or the call
				// is interrupted by a signal handler (explicitly retried in the loop above),
				// or the timeout expires.
				t.Errorf("Select: slept for %v, expected %v", took, dur)
			} else {
				t.Logf("Select: slept for %v, requested %v", took, dur)
			}
		}
	})

	t.Run("should return 1 if a given FD has data", func(t *testing.T) {
		rr, ww, err := os.Pipe()
		require.NoError(t, err)
		defer rr.Close()
		defer ww.Close()

		_, err = ww.Write([]byte("TEST"))
		require.NoError(t, err)

		rFdSet := &platform.FdSet{}
		fd := int(rr.Fd())
		rFdSet.Set(fd)

		for {
			n, err := _select(fd+1, rFdSet, nil, nil, nil)
			if err == sys.EINTR {
				t.Log("Select interrupted")
				continue
			}
			require.NoError(t, err)
			require.Equal(t, 1, n)
			break
		}
	})
}
