package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestFeatures_ZeroIsInvalid reminds maintainers that a bitset cannot use zero as a flag!
// This is why we start iota with 1.
func TestFeatures_ZeroIsInvalid(t *testing.T) {
	f := Features(0)
	f = f.Set(0, true)
	require.False(t, f.Get(0))
}

// TestFeatures tests the bitset works as expected
func TestFeatures(t *testing.T) {
	tests := []struct {
		name    string
		feature Features
	}{
		{
			name:    "one is the smallest flag",
			feature: 1,
		},
		{
			name:    "63 is the largest feature flag", // because uint64
			feature: 1 << 63,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := Features(0)

			// Defaults to false
			require.False(t, f.Get(tc.feature))

			// Set true makes it true
			f = f.Set(tc.feature, true)
			require.True(t, f.Get(tc.feature))

			// Set false makes it false again
			f = f.Set(tc.feature, false)
			require.False(t, f.Get(tc.feature))
		})
	}
}

func TestFeatures_String(t *testing.T) {
	tests := []struct {
		name     string
		feature  Features
		expected string
	}{
		{name: "none", feature: 0, expected: ""},
		{name: "mutable-global", feature: FeatureMutableGlobal, expected: "mutable-global"},
		{name: "sign-extension-ops", feature: FeatureSignExtensionOps, expected: "sign-extension-ops"},
		{name: "multi-value", feature: FeatureMultiValue, expected: "multi-value"},
		{name: "simd", feature: FeatureSIMD, expected: "simd"},
		{name: "features", feature: FeatureMutableGlobal | FeatureMultiValue, expected: "multi-value|mutable-global"},
		{name: "undefined", feature: 1 << 63, expected: ""},
		{name: "2.0", feature: Features20220419,
			expected: "bulk-memory-operations|multi-value|mutable-global|" +
				"nontrapping-float-to-int-conversion|reference-types|sign-extension-ops|simd"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.feature.String())
		})
	}
}

func TestFeatures_Require(t *testing.T) {
	tests := []struct {
		name        string
		feature     Features
		expectedErr string
	}{
		{name: "none", feature: 0, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "mutable-global", feature: FeatureMutableGlobal},
		{name: "sign-extension-ops", feature: FeatureSignExtensionOps, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "multi-value", feature: FeatureMultiValue, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "undefined", feature: 1 << 63, expectedErr: "feature \"mutable-global\" is disabled"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := tc.feature.Require(FeatureMutableGlobal)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
