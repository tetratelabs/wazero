package net

import (
	"fmt"
	"net"
	"syscall"

	"github.com/tetratelabs/wazero/internal/fsapi"
)

// ConfigKey is a context.Context Value key. Its associated value should be a Config.
type ConfigKey struct{}

// Config is an internal struct meant to implement
// the interface in experimental/net/Config.
type Config struct {
	// TCPListeners is a slice of the configured host:port pairs.
	TCPListeners []TCPListener
}

// TCPListener is a host:port pair to pre-open.
type TCPListener struct {
	// Host is the host name for this listener.
	Host string
	// Port is the port number for this listener.
	Port int
}

// WithTCPListener implements the method of the same name in experimental/net/Config.
//
// However, to avoid cyclic dependencies, this is returning the *Config in this scope.
// The interface is implemented in experimental/net/Config via delegation.
func (c *Config) WithTCPListener(host string, port int) *Config {
	ret := c.clone()
	ret.TCPListeners = append(ret.TCPListeners, TCPListener{host, port})
	return &ret
}

// Makes a deep copy of this netConfig.
func (c *Config) clone() Config {
	ret := *c
	ret.TCPListeners = make([]TCPListener, 0, len(c.TCPListeners))
	ret.TCPListeners = append(ret.TCPListeners, c.TCPListeners...)
	return ret
}

// BuildTCPListeners build listeners from the current configuration.
func (c *Config) BuildTCPListeners() (tcpListeners []*net.TCPListener, err error) {
	for _, tcpAddr := range c.TCPListeners {
		var ln net.Listener
		ln, err = net.Listen("tcp", tcpAddr.String())
		if err != nil {
			break
		}
		if tcpln, ok := ln.(*net.TCPListener); ok {
			tcpListeners = append(tcpListeners, tcpln)
		}
	}
	if err != nil {
		// An error occurred, cleanup.
		for _, l := range tcpListeners {
			_ = l.Close() // Ignore errors, we are already cleaning.
		}
		tcpListeners = nil
	}
	return
}

func (t TCPListener) String() string {
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}

type Sock interface {
	fsapi.File

	Accept() (Conn, syscall.Errno)
}

type Conn interface {
	fsapi.File

	// Recvfrom only supports the flag sysfs.MSG_PEEK
	Recvfrom(p []byte, flags int) (n int, errno syscall.Errno)

	Shutdown(how int) syscall.Errno
}
