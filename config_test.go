package wazero

import (
	"testing"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
)

func TestRuntimeConfig_Features(t *testing.T) {
	tests := []struct {
		name          string
		feature       internalwasm.Features
		expectDefault bool
		setFeature    func(*RuntimeConfig, bool) *RuntimeConfig
	}{
		{
			name:          "mutable-global",
			feature:       internalwasm.FeatureMutableGlobal,
			expectDefault: true,
			setFeature: func(c *RuntimeConfig, v bool) *RuntimeConfig {
				return c.WithFeatureMutableGlobal(v)
			},
		},
		{
			name:          "sign-extension-ops",
			feature:       internalwasm.FeatureSignExtensionOps,
			expectDefault: false,
			setFeature: func(c *RuntimeConfig, v bool) *RuntimeConfig {
				return c.WithFeatureSignExtensionOps(v)
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			c := NewRuntimeConfig()
			require.Equal(t, tc.expectDefault, c.enabledFeatures.Get(tc.feature))

			// Set to false even if it was initially false.
			c = tc.setFeature(c, false)
			require.False(t, c.enabledFeatures.Get(tc.feature))

			// Set true makes it true
			c = tc.setFeature(c, true)
			require.True(t, c.enabledFeatures.Get(tc.feature))

			// Set false makes it false again
			c = tc.setFeature(c, false)
			require.False(t, c.enabledFeatures.Get(tc.feature))
		})
	}
}

// TODO: test buildSysContext
