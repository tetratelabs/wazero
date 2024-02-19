package sysfs

import (
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestPoll_Windows(t *testing.T) {
	type result struct {
		n   int
		err sys.Errno
	}

	pollToChannel := func(fd uintptr, timeoutMillis int32, ch chan result) {
		r := result{}
		fds := []pollFd{{fd: fd, events: _POLLIN}}
		r.n, r.err = _poll(fds, timeoutMillis)
		ch <- r
		close(ch)
	}

	t.Run("poll returns sys.ENOSYS when n == 0 and timeoutMillis is negative", func(t *testing.T) {
		n, errno := _poll(nil, -1)
		require.Equal(t, -1, n)
		require.EqualErrno(t, sys.ENOSYS, errno)
	})

	t.Run("peekNamedPipe should report the correct state of incoming data in the pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()
		rh := syscall.Handle(r.Fd())

		// Ensure the pipe has no data.
		n, err := peekNamedPipe(rh)
		require.Zero(t, err)
		require.Zero(t, n)

		// Write to the channel.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = write(w, msg)
		require.EqualErrno(t, 0, err)

		// Ensure the pipe has data.
		n, err = peekNamedPipe(rh)
		require.Zero(t, err)
		require.Equal(t, 6, int(n))
	})

	t.Run("peekPipes should return an error on invalid handle", func(t *testing.T) {
		fds := []pollFd{{fd: uintptr(syscall.InvalidHandle)}}
		_, err := peekPipes(fds)
		require.EqualErrno(t, sys.EBADF, err)
	})

	t.Run("peekAll should return an error on invalid handle", func(t *testing.T) {
		fds := []pollFd{{fd: uintptr(syscall.InvalidHandle)}}
		_, _, err := peekAll(fds, nil)
		require.EqualErrno(t, sys.EBADF, err)
	})

	t.Run("poll should return successfully with a regular file", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "test")
		require.NoError(t, err)
		defer f.Close()

		fds := []pollFd{{fd: f.Fd()}}

		n, errno := _poll(fds, 0)
		require.Zero(t, errno)
		require.Equal(t, 1, n)
	})

	t.Run("peekAll should return successfully with a pipe", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		fds := []pollFd{{fd: r.Fd()}}

		npipes, nsockets, errno := peekAll(fds, nil)
		require.Zero(t, errno)
		require.Equal(t, 0, npipes)
		require.Equal(t, 0, nsockets)

		w.Write([]byte("wazero"))
		npipes, nsockets, errno = peekAll(fds, nil)
		require.Zero(t, errno)
		require.Equal(t, 1, npipes)
		require.Equal(t, 0, nsockets)
	})

	t.Run("peekAll should return successfully with a socket", func(t *testing.T) {
		listen, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listen.Close()

		conn, err := listen.(*net.TCPListener).SyscallConn()
		require.NoError(t, err)

		fds := []pollFd{}
		conn.Control(func(fd uintptr) {
			fds = append(fds, pollFd{fd: fd, events: _POLLIN})
		})

		npipes, nsockets, errno := peekAll(nil, fds)
		require.Zero(t, errno)
		require.Equal(t, 0, npipes)
		require.Equal(t, 0, nsockets)

		tcpAddr, err := net.ResolveTCPAddr("tcp", listen.Addr().String())
		require.NoError(t, err)
		tcp, err := net.DialTCP("tcp", nil, tcpAddr)
		require.NoError(t, err)
		tcp.Write([]byte("wazero"))

		conn.Control(func(fd uintptr) {
			fds[0].fd = fd
		})
		npipes, nsockets, errno = peekAll(nil, fds)
		require.Zero(t, errno)
		require.Equal(t, 0, npipes)
		require.Equal(t, 1, nsockets)
	})

	t.Run("poll should return immediately when duration is zero (no data)", func(t *testing.T) {
		r, w, err := os.Pipe()
		defer r.Close()
		defer w.Close()

		require.NoError(t, err)
		fds := []pollFd{{fd: r.Fd(), events: _POLLIN}}
		n, err := _poll(fds, 0)
		require.Zero(t, err)
		require.Zero(t, n)
	})

	t.Run("poll should return immediately when duration is zero (data)", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()
		fds := []pollFd{{fd: r.Fd(), events: _POLLIN}}

		// Write to the channel immediately.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = write(w, msg)
		require.EqualErrno(t, 0, err)

		// Verify that the write is reported.
		n, err := _poll(fds, 0)
		require.Zero(t, err)
		require.Equal(t, 1, n)
	})

	t.Run("poll should wait forever when duration is nil (no writes)", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		ch := make(chan result, 1)
		go pollToChannel(r.Fd(), -1, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(500 * time.Millisecond)
		require.Equal(t, 0, len(ch))
	})

	t.Run("poll should wait forever when duration is nil", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		ch := make(chan result, 1)
		go pollToChannel(r.Fd(), -1, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(100 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		// Write a message to the pipe.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = write(w, msg)
		require.EqualErrno(t, 0, err)

		// Ensure that the write occurs (panic after an arbitrary timeout).
		select {
		case <-time.After(500 * time.Millisecond):
			t.Fatal("unreachable!")
		case r := <-ch:
			require.Zero(t, r.err)
			require.NotEqual(t, 0, r.n)
		}
	})

	t.Run("poll should wait for the given duration", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		ch := make(chan result, 1)
		go pollToChannel(r.Fd(), 500, ch)

		// Wait a little, then ensure no writes occurred.
		<-time.After(100 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		// Write a message to the pipe.
		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = write(w, msg)
		require.EqualErrno(t, 0, err)

		// Ensure that the write occurs before the timer expires.
		select {
		case <-time.After(500 * time.Millisecond):
			panic("no data!")
		case r := <-ch:
			require.Zero(t, r.err)
			require.Equal(t, 1, r.n)
		}
	})

	t.Run("poll should timeout after the given duration", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		ch := make(chan result, 1)
		go pollToChannel(r.Fd(), 200, ch)

		// Ensure that the timer has expired.
		res := <-ch
		require.Zero(t, res.err)
		require.Zero(t, res.n)
	})

	t.Run("poll should return when a write occurs before the given duration", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()
		defer w.Close()

		ch := make(chan result, 1)
		go pollToChannel(r.Fd(), 800, ch)

		<-time.After(300 * time.Millisecond)
		require.Equal(t, 0, len(ch))

		msg, err := syscall.ByteSliceFromString("test\n")
		require.NoError(t, err)
		_, err = write(w, msg)
		require.EqualErrno(t, 0, err)

		res := <-ch
		require.Zero(t, res.err)
		require.Equal(t, 1, res.n)
	})

	t.Run("poll should return when a regular file is given", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "ex")
		require.NoError(t, err)
		defer f.Close()

		require.NoError(t, err)
		fds := []pollFd{{fd: f.Fd(), events: _POLLIN}}
		n, errno := _poll(fds, 0)
		require.Zero(t, errno)
		require.Equal(t, 1, n)
	})
}
