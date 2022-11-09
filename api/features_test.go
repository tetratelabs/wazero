package api

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestCoreFeatures_ZeroIsInvalid reminds maintainers that a bitset cannot use zero as a flag!
// This is why we start iota with 1.
func TestCoreFeatures_ZeroIsInvalid(t *testing.T) {
	f := CoreFeatures(0)
	f = f.SetEnabled(0, true)
	require.False(t, f.IsEnabled(0))
}

// TestCoreFeatures tests the bitset works as expected
func TestCoreFeatures(t *testing.T) {
	tests := []struct {
		name    string
		feature CoreFeatures
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
			f := CoreFeatures(0)

			// Defaults to false
			require.False(t, f.IsEnabled(tc.feature))

			// Set true makes it true
			f = f.SetEnabled(tc.feature, true)
			require.True(t, f.IsEnabled(tc.feature))

			// Set false makes it false again
			f = f.SetEnabled(tc.feature, false)
			require.False(t, f.IsEnabled(tc.feature))
		})
	}
}

func TestCoreFeatures_String(t *testing.T) {
	tests := []struct {
		name     string
		feature  CoreFeatures
		expected string
	}{
		{name: "none", feature: 0, expected: ""},
		{name: "mutable-global", feature: CoreFeatureMutableGlobal, expected: "mutable-global"},
		{name: "sign-extension-ops", feature: CoreFeatureSignExtensionOps, expected: "sign-extension-ops"},
		{name: "multi-value", feature: CoreFeatureMultiValue, expected: "multi-value"},
		{name: "simd", feature: CoreFeatureSIMD, expected: "simd"},
		{name: "features", feature: CoreFeatureMutableGlobal | CoreFeatureMultiValue, expected: "multi-value|mutable-global"},
		{name: "undefined", feature: 1 << 63, expected: ""},
		{
			name: "2.0", feature: CoreFeaturesV2,
			expected: "bulk-memory-operations|multi-value|mutable-global|" +
				"nontrapping-float-to-int-conversion|reference-types|sign-extension-ops|simd",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.feature.String())
		})
	}
}

func TestCoreFeatures_Require(t *testing.T) {
	tests := []struct {
		name        string
		feature     CoreFeatures
		expectedErr string
	}{
		{name: "none", feature: 0, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "mutable-global", feature: CoreFeatureMutableGlobal},
		{name: "sign-extension-ops", feature: CoreFeatureSignExtensionOps, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "multi-value", feature: CoreFeatureMultiValue, expectedErr: "feature \"mutable-global\" is disabled"},
		{name: "undefined", feature: 1 << 63, expectedErr: "feature \"mutable-global\" is disabled"},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := tc.feature.RequireEnabled(CoreFeatureMutableGlobal)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
