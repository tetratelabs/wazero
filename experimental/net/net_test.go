package net_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/experimental/net"
	internalnet "github.com/tetratelabs/wazero/internal/net"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestWithNetConfig(t *testing.T) {
	tests := []struct {
		name     string
		netCfg   net.Config
		expected bool
	}{
		{
			name:     "returns input when netCfg nil",
			expected: false,
		},
		{
			name:     "returns input when netCfg empty",
			netCfg:   net.NewConfig(),
			expected: false,
		},
		{
			name:     "decorates with netCfg",
			netCfg:   net.NewConfig().WithTCPListener("", 0),
			expected: true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if decorated := net.WithConfig(testCtx, tc.netCfg); tc.expected {
				require.NotNil(t, decorated.Value(internalnet.ConfigKey{}))
			} else {
				require.Same(t, testCtx, decorated)
			}
		})
	}
}
