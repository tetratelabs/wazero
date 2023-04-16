package platform

import (
	"runtime"
	"testing"
)

func TestFdSet(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("not supported")
	}

	var fdSet FdSet
	fdSet.Zero()
	for fd := 0; fd < nfdbits; fd++ {
		if fdSet.IsSet(fd) {
			t.Fatalf("Zero did not clear fd %d", fd)
		}
		fdSet.Set(fd)
	}

	for fd := 0; fd < nfdbits; fd++ {
		if !fdSet.IsSet(fd) {
			t.Fatalf("IsSet(%d): expected true, got false", fd)
		}
	}

	fdSet.Zero()
	for fd := 0; fd < nfdbits; fd++ {
		if fdSet.IsSet(fd) {
			t.Fatalf("Zero did not clear fd %d", fd)
		}
	}

	for fd := 1; fd < nfdbits; fd += 2 {
		fdSet.Set(fd)
	}

	for fd := 0; fd < nfdbits; fd++ {
		if fd&0x1 == 0x1 {
			if !fdSet.IsSet(fd) {
				t.Fatalf("IsSet(%d): expected true, got false", fd)
			}
		} else {
			if fdSet.IsSet(fd) {
				t.Fatalf("IsSet(%d): expected false, got true", fd)
			}
		}
	}

	for fd := 1; fd < nfdbits; fd += 2 {
		fdSet.Clear(fd)
	}

	for fd := 0; fd < nfdbits; fd++ {
		if fdSet.IsSet(fd) {
			t.Fatalf("Clear(%d) did not clear fd", fd)
		}
	}
}
