package net

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/internal/net"
)

// NetConfig configures the host to set up host:port listeners
// that the embedding host will allow the wasm guest to access.
//
// These will result in preopened socket file-descriptors at run-time.
type NetConfig interface {
	// WithTCPListener configures the host to set up the given host:port listener.
	WithTCPListener(host string, port uint16) NetConfig
	// WithTCPListenerFromString parses a string of the form "host:port"
	// into a host:port pair.
	WithTCPListenerFromString(conn string) (NetConfig, error)
}

// NewNetConfig returns a NetConfig that can be used for configuring module instantiation.
func NewNetConfig() NetConfig {
	return &internalNetConfig{c: &net.NetConfig{}}
}

// internalNetConfig is a wrapper to internal/net.NetConfig to avoid circular dependencies.
// It implements the NetConfig via delegation.
type internalNetConfig struct {
	c *net.NetConfig
}

// WithTCPListener implements the method of the same name in NetConfig.
func (c *internalNetConfig) WithTCPListener(host string, port uint16) NetConfig {
	cNew := c.c.WithTCPListener(host, port)
	return &internalNetConfig{cNew}
}

// WithTCPListenerFromString implements the method of the same name in NetConfig.
func (c *internalNetConfig) WithTCPListenerFromString(conn string) (NetConfig, error) {
	idx := strings.LastIndexByte(conn, ':')
	if idx < 0 {
		return nil, errors.New("invalid connection string")
	}
	port, err := strconv.Atoi(conn[idx+1:])
	if err != nil {
		return nil, err
	}
	return c.WithTCPListener(conn[:idx], uint16(port)), nil
}

// WithNetConfig registers the given NetConfig into the given context.Context.
func WithNetConfig(ctx context.Context, netCfg NetConfig) context.Context {
	return context.WithValue(ctx, net.NetConfigKey{}, netCfg.(*internalNetConfig).c)
}
