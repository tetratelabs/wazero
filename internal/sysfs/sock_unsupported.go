//go:build !linux && !darwin && !windows

package sysfs

import (
	"net"

	socketapi "github.com/tetratelabs/wazero/internal/sock"
)

// MSG_PEEK is a filler value.
const MSG_PEEK = 0x2

func newTCPListenerFile(tl *net.TCPListener) socketapi.TCPSock {
	return &baseSockFile{}
}
