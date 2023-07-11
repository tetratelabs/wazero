package sysfs

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSelect_Windows(t *testing.T) {
	type result struct {
		n     int
		fdSet platform.FdSet
		err   sys.Errno
	}

	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handleAsFdSet := func(readHandle syscall.Handle) *platform.FdSet {
		var fdSet platform.FdSet
		fdSet.Set(int(readHandle))
		return &fdSet
	}

	pollToChannel := func(readHandle syscall.Handle, duration *time.Duration, ch chan result) {
		r := result{}
		fdSet := handleAsFdSet(readHandle)
		r.n, r.err = selectAllHandles(testCtx, fdSet, nil, nil, duration)
		r.fdSet = *fdSet
		ch <- r
		close(ch)
	}

	t.Run("peekNamedPipe should report the correct state of incoming data in the pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		// Ensure the pipe has no data.
		n, err := peekNamedPipe(rh)
		require.Zero(t, err)
		require.Zero(t, n)

		// Write to the channel.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Ensure the pipe has data.
		n, err = peekNamedPipe(rh)
		require.Zero(t, err)
		require.Equal(t, 6, int(n))
	})

	t.Run("selectAllHandles should return immediately when duration is nil (no data)", func(t *testing.T) {
		r, _, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		d := time.Duration(0)
		fdSet := handleAsFdSet(rh)
		n, err := selectAllHandles(testCtx, fdSet, nil, nil, &d)
		require.Zero(t, err)
		require.Zero(t, n)
		require.Zero(t, fdSet.Pipes().Count())
	})

	t.Run("selectAllHandles should return immediately when duration is nil (data)", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := handleAsFdSet(syscall.Handle(r.Fd()))
		wh := syscall.Handle(w.Fd())

		// Write to the channel immediately.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Verify that the write is reported.
		d := time.Duration(0)
		n, err := selectAllHandles(testCtx, rh, nil, nil, &d)
		require.Zero(t, err)
		require.NotEqual(t, 0, n)
		require.Equal(t, syscall.Handle(r.Fd()), rh.Pipes().Get(0))
	})

	t.Run("selectAllHandles should wait forever when duration is nil", func(t *testing.T) {
		r, _, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())

		ch := make(chan result, 1)
		go pollToChannel(rh, nil, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(500 * time.Millisecond)
		require.Equal(t, 0, len(ch))
	})

	t.Run("selectAllHandles should wait forever when duration is nil", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		ch := make(chan result, 1)
		go pollToChannel(rh, nil, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(100 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		// Write a message to the pipe.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Ensure that the write occurs (panic after an arbitrary timeout).
		select {
		case <-time.After(500 * time.Millisecond):
			t.Fatal("unreachable!")
		case r := <-ch:
			require.Zero(t, r.err)
			require.NotEqual(t, 0, r.n)
			require.Equal(t, rh, r.fdSet.Pipes().Get(0))
		}
	})

	t.Run("selectAllHandles should wait for the given duration", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		d := 500 * time.Millisecond
		ch := make(chan result, 1)
		go pollToChannel(rh, &d, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(100 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		// Write a message to the pipe.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Ensure that the write occurs before the timer expires.
		select {
		case <-time.After(500 * time.Millisecond):
			panic("no data!")
		case r := <-ch:
			require.Zero(t, r.err)
			require.Equal(t, 1, r.n)
			require.Equal(t, rh, r.fdSet.Pipes().Get(0))
		}
	})

	t.Run("selectAllHandles should timeout after the given duration", func(t *testing.T) {
		r, _, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())

		d := 200 * time.Millisecond
		ch := make(chan result, 1)
		go pollToChannel(rh, &d, ch)

		// Wait a little, then ensure a message has been written to the channel.
		<-time.After(300 * time.Millisecond)
		require.Equal(t, 1, len(ch))

		// Ensure that the timer has expired.
		res := <-ch
		require.Zero(t, res.err)
		require.Zero(t, res.n)
		require.Zero(t, res.fdSet.Pipes().Count())
	})

	t.Run("selectAllHandles should return when a write occurs before the given duration", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		d := 600 * time.Millisecond
		ch := make(chan result, 1)
		go pollToChannel(rh, &d, ch)

		<-time.After(300 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		res := <-ch
		require.Zero(t, res.err)
		require.Equal(t, 1, res.n)
		require.Equal(t, rh, res.fdSet.Pipes().Get(0))
	})

	t.Run("selectAllHandles should return when a regular file is given", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "ex")
		defer f.Close()
		require.NoError(t, err)
		fh := syscall.Handle(f.Fd())
		fdSet := handleAsFdSet(fh)
		d := time.Duration(0)
		n, errno := selectAllHandles(testCtx, fdSet, nil, nil, &d)
		require.Zero(t, errno)
		require.Equal(t, 1, n)
		require.Equal(t, fh, fdSet.Regular().Get(0))
	})
}
