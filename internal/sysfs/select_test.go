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
	t.Run("should return immediately with no fds and timeoutNanos 0", func(t *testing.T) {
		for {
			timeoutNanos := int32(0)
			ready, errno := _select(0, nil, nil, nil, timeoutNanos)
			if errno == sys.EINTR {
				t.Logf("Select interrupted")
				continue
			}
			require.EqualErrno(t, 0, errno)
			require.False(t, ready)
			break
		}
	})

	t.Run("should wait for the given duration", func(t *testing.T) {
		timeoutNanos := int32(250 * time.Millisecond)
		var took time.Duration
		for {
			// On some platforms (e.g. Linux), the passed-in timeval is
			// updated by select(2). We are not accounting for this
			// in our implementation.
			start := time.Now()
			ready, errno := _select(0, nil, nil, nil, timeoutNanos)
			took = time.Since(start)
			if errno == sys.EINTR {
				t.Logf("Select interrupted after %v", took)
				continue
			}
			require.EqualErrno(t, 0, errno)
			require.False(t, ready)
			break
		}

		// On some platforms the actual timeout might be arbitrarily
		// less than requested.
		if tookNanos := int32(took.Nanoseconds()); tookNanos < timeoutNanos {
			if runtime.GOOS == "linux" {
				// Linux promises to only return early if a file descriptor
				// becomes ready (not applicable here), or the call
				// is interrupted by a signal handler (explicitly retried in the loop above),
				// or the timeout expires.
				t.Errorf("Select: slept for %v, expected %v", tookNanos, timeoutNanos)
			} else {
				t.Logf("Select: slept for %v, requested %v", tookNanos, timeoutNanos)
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
			ready, errno := _select(fd+1, rFdSet, nil, nil, -1)
			if errno == sys.EINTR {
				t.Log("Select interrupted")
				continue
			}
			require.EqualErrno(t, 0, errno)
			require.True(t, ready)
			break
		}
	})
}
