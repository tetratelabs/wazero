package net

import (
	"context"

	"github.com/tetratelabs/wazero/internal/net"
)

// Config configures the host to open TCP sockets and allows guest access to
// them.
//
// Instantiating a module with listeners results in pre-opened sockets
// associated with file-descriptors numerically after pre-opened files.
type Config interface {
	// WithTCPListener configures the host to set up the given host:port listener.
	WithTCPListener(host string, port int) Config
}

// NewConfig returns a Config for module instantiation.
func NewConfig() Config {
	return &internalNetConfig{c: &net.Config{}}
}

// internalNetConfig delegates to internal/net.Config to avoid circular
// dependencies.
type internalNetConfig struct {
	c *net.Config
}

// WithTCPListener implements Config.WithTCPListener
func (c *internalNetConfig) WithTCPListener(host string, port int) Config {
	cNew := c.c.WithTCPListener(host, port)
	return &internalNetConfig{cNew}
}

// WithConfig registers the given Config into the given context.Context.
func WithConfig(ctx context.Context, config Config) context.Context {
	if config, ok := config.(*internalNetConfig); ok && len(config.c.TCPListeners) > 0 {
		return context.WithValue(ctx, net.ConfigKey{}, config.c)
	}
	return ctx
}
