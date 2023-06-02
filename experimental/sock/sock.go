package sock

import (
	"context"

	"github.com/tetratelabs/wazero/internal/sock"
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
	return &internalSockConfig{c: &sock.Config{}}
}

// internalSockConfig delegates to internal/sock.Config to avoid circular
// dependencies.
type internalSockConfig struct {
	c *sock.Config
}

// WithTCPListener implements Config.WithTCPListener
func (c *internalSockConfig) WithTCPListener(host string, port int) Config {
	cNew := c.c.WithTCPListener(host, port)
	return &internalSockConfig{cNew}
}

// WithConfig registers the given Config into the given context.Context.
func WithConfig(ctx context.Context, config Config) context.Context {
	if config, ok := config.(*internalSockConfig); ok && len(config.c.TCPAddresses) > 0 {
		return context.WithValue(ctx, sock.ConfigKey{}, config.c)
	}
	return ctx
}
