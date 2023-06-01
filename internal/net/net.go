package net

import (
	"fmt"
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

func (t TCPListener) String() string {
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}
