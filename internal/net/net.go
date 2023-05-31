package net

import (
	"fmt"
)

// NetConfig is an internal struct meant to implement
// the interface in experimental/net/NetConfig.
type NetConfig struct {
	// TCPListeners is a slice of the configured host:port pairs.
	TCPListeners []TCPListener
}

// WithTCPListener implements the method of the same name in experimental/net/NetConfig.
//
// However, to avoid cyclic dependencies, this is returning the *NetConfig in this scope.
// The interface is implemented in experimental/net/NetConfig via delegation.
func (c *NetConfig) WithTCPListener(host string, port uint16) *NetConfig {
	ret := c.clone()
	ret.TCPListeners = append(ret.TCPListeners, TCPListener{host, port})
	return &ret
}

// Makes a deep copy of this netConfig.
func (c *NetConfig) clone() NetConfig {
	ret := *c
	ret.TCPListeners = make([]TCPListener, 0, len(c.TCPListeners))
	ret.TCPListeners = append(ret.TCPListeners, c.TCPListeners...)
	return ret
}

// NetConfigKey is a context.Context Value key. Its associated value should be a NetConfig.
type NetConfigKey struct{}

// TCPListener is a host:port pair to pre-open.
type TCPListener struct {
	// Host is the host name for this listener.
	Host string
	// Port is the port number for this listener.
	Port uint16
}

func (t TCPListener) String() string {
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}
