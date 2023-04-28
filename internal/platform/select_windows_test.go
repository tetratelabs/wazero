package platform

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSelect_Windows(t *testing.T) {
	type result struct {
		hasData bool
		err     error
	}

	testCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pollToChannel := func(readHandle syscall.Handle, duration *time.Duration, ch chan result) {
		r := result{}
		r.hasData, r.err = pollNamedPipe(testCtx, readHandle, duration)
		ch <- r
		close(ch)
	}

	t.Run("peekNamedPipe should report the correct state of incoming data in the pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		// Ensure the pipe has data.
		hasData, err := peekNamedPipe(rh)
		require.NoError(t, err)
		require.False(t, hasData)

		// Write to the channel.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Ensure the pipe has data.
		hasData, err = peekNamedPipe(rh)
		require.NoError(t, err)
		require.True(t, hasData)
	})

	t.Run("pollNamedPipe should return immediately when duration is nil (no data)", func(t *testing.T) {
		r, _, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		d := time.Duration(0)
		hasData, err := pollNamedPipe(testCtx, rh, &d)
		require.NoError(t, err)
		require.False(t, hasData)
	})

	t.Run("pollNamedPipe should return immediately when duration is nil (data)", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())
		wh := syscall.Handle(w.Fd())

		// Write to the channel immediately.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = syscall.Write(wh, msg)
		require.NoError(t, err)

		// Verify that the write is reported.
		d := time.Duration(0)
		hasData, err := pollNamedPipe(testCtx, rh, &d)
		require.NoError(t, err)
		require.True(t, hasData)
	})

	t.Run("pollNamedPipe should wait forever when duration is nil", func(t *testing.T) {
		r, _, err := os.Pipe()
		require.NoError(t, err)
		rh := syscall.Handle(r.Fd())

		ch := make(chan result, 1)
		go pollToChannel(rh, nil, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(500 * time.Millisecond)
		require.Equal(t, 0, len(ch))
	})

	t.Run("pollNamedPipe should wait forever when duration is nil", func(t *testing.T) {
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
			panic("unreachable!")
		case r := <-ch:
			require.NoError(t, r.err)
			require.True(t, r.hasData)
		}
	})

	t.Run("pollNamedPipe should wait for the given duration", func(t *testing.T) {
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
			require.NoError(t, r.err)
			require.True(t, r.hasData)
		}
	})

	t.Run("pollNamedPipe should timeout after the given duration", func(t *testing.T) {
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
		require.NoError(t, res.err)
		require.False(t, res.hasData)
	})

	t.Run("pollNamedPipe should return when a write occurs before the given duration", func(t *testing.T) {
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
		require.NoError(t, res.err)
		require.True(t, res.hasData)
	})
}
