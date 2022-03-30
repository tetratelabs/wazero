package wazero

import (
	"context"
	"io"
	"math"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
)

func TestRuntimeConfig(t *testing.T) {
	tests := []struct {
		name     string
		with     func(*RuntimeConfig) *RuntimeConfig
		expected *RuntimeConfig
	}{
		{
			name: "WithContext",
			with: func(c *RuntimeConfig) *RuntimeConfig {
				return c.WithContext(context.TODO())
			},
			expected: &RuntimeConfig{
				ctx: context.TODO(),
			},
		},
		{
			name: "WithContext - nil",
			with: func(c *RuntimeConfig) *RuntimeConfig {
				return c.WithContext(nil) //nolint
			},
			expected: &RuntimeConfig{
				ctx: context.Background(),
			},
		},
		{
			name: "WithMemoryMaxPages",
			with: func(c *RuntimeConfig) *RuntimeConfig {
				return c.WithMemoryMaxPages(1)
			},
			expected: &RuntimeConfig{
				memoryMaxPages: 1,
			},
		},
		{
			name: "mutable-global",
			with: func(c *RuntimeConfig) *RuntimeConfig {
				return c.WithFeatureMutableGlobal(true)
			},
			expected: &RuntimeConfig{
				enabledFeatures: internalwasm.FeatureMutableGlobal,
			},
		},
		{
			name: "sign-extension-ops",
			with: func(c *RuntimeConfig) *RuntimeConfig {
				return c.WithFeatureSignExtensionOps(true)
			},
			expected: &RuntimeConfig{
				enabledFeatures: internalwasm.FeatureSignExtensionOps,
			},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := &RuntimeConfig{}
			rc := tc.with(input)
			require.Equal(t, tc.expected, rc)
			// The source wasn't modified
			require.Equal(t, &RuntimeConfig{}, input)
		})
	}
}

func TestRuntimeConfig_FeatureToggle(t *testing.T) {
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

func TestSysConfig_toSysContext(t *testing.T) {
	testFS := fstest.MapFS{}
	testFS2 := fstest.MapFS{}

	tests := []struct {
		name     string
		input    *SysConfig
		expected *internalwasm.SysContext
	}{
		{
			name:  "empty",
			input: NewSysConfig(),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // openedFiles
			),
		},
		{
			name:  "WithArgs",
			input: NewSysConfig().WithArgs("a", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"a", "bc"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // openedFiles
			),
		},
		{
			name:  "WithArgs - empty ok", // Particularly argv[0] can be empty, and we have no rules about others.
			input: NewSysConfig().WithArgs("", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,     // max
				[]string{"", "bc"}, // args
				nil,                // environ
				nil,                // stdin
				nil,                // stdout
				nil,                // stderr
				nil,                // openedFiles
			),
		},
		{
			name:  "WithArgs - second call overwrites",
			input: NewSysConfig().WithArgs("a", "bc").WithArgs("bc", "a"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"bc", "a"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // openedFiles
			),
		},
		{
			name:  "WithEnv",
			input: NewSysConfig().WithEnv("a", "b"),
			expected: requireSysContext(t,
				math.MaxUint32,  // max
				nil,             // args
				[]string{"a=b"}, // environ
				nil,             // stdin
				nil,             // stdout
				nil,             // stderr
				nil,             // openedFiles
			),
		},
		{
			name:  "WithEnv - empty value",
			input: NewSysConfig().WithEnv("a", ""),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				[]string{"a="}, // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // openedFiles
			),
		},
		{
			name:  "WithEnv twice",
			input: NewSysConfig().WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // openedFiles
			),
		},
		{
			name:  "WithEnv overwrites",
			input: NewSysConfig().WithEnv("a", "bc").WithEnv("c", "de").WithEnv("a", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,           // max
				nil,                      // args
				[]string{"a=de", "c=de"}, // environ
				nil,                      // stdin
				nil,                      // stdout
				nil,                      // stderr
				nil,                      // openedFiles
			),
		},

		{
			name:  "WithEnv twice",
			input: NewSysConfig().WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // openedFiles
			),
		},
		{
			name:  "WithFS",
			input: NewSysConfig().WithFS(testFS),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				map[uint32]*internalwasm.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS},
					4: {Path: ".", FS: testFS},
				},
			),
		},
		{
			name:  "WithFS - overwrites",
			input: NewSysConfig().WithFS(testFS).WithFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				map[uint32]*internalwasm.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS2},
					4: {Path: ".", FS: testFS2},
				},
			),
		},
		{
			name:  "WithWorkDirFS",
			input: NewSysConfig().WithWorkDirFS(testFS),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				map[uint32]*internalwasm.FileEntry{ // openedFiles
					3: {Path: ".", FS: testFS},
				},
			),
		},
		{
			name:  "WithFS and WithWorkDirFS",
			input: NewSysConfig().WithFS(testFS).WithWorkDirFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				map[uint32]*internalwasm.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS},
					4: {Path: ".", FS: testFS2},
				},
			),
		},
		{
			name:  "WithWorkDirFS and WithFS",
			input: NewSysConfig().WithWorkDirFS(testFS).WithFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				map[uint32]*internalwasm.FileEntry{ // openedFiles
					3: {Path: ".", FS: testFS},
					4: {Path: "/", FS: testFS2},
				},
			),
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sys, err := tc.input.toSysContext()
			require.NoError(t, err)
			require.Equal(t, tc.expected, sys)
		})
	}
}

func TestSysConfig_toSysContext_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       *SysConfig
		expectedErr string
	}{
		{
			name:        "WithArgs - arg contains NUL",
			input:       NewSysConfig().WithArgs("", string([]byte{'a', 0})),
			expectedErr: "args invalid: contains NUL character",
		},
		{
			name:        "WithEnv - key contains NUL",
			input:       NewSysConfig().WithEnv(string([]byte{'a', 0}), "a"),
			expectedErr: "environ invalid: contains NUL character",
		},
		{
			name:        "WithEnv - value contains NUL",
			input:       NewSysConfig().WithEnv("a", string([]byte{'a', 0})),
			expectedErr: "environ invalid: contains NUL character",
		},
		{
			name:        "WithEnv - key contains equals",
			input:       NewSysConfig().WithEnv("a=", "a"),
			expectedErr: "environ invalid: key contains '=' character",
		},
		{
			name:        "WithEnv - empty key",
			input:       NewSysConfig().WithEnv("", "a"),
			expectedErr: "environ invalid: empty key",
		},
		{
			name:        "WithFS - nil",
			input:       NewSysConfig().WithFS(nil),
			expectedErr: "FS for / is nil",
		},
		{
			name:        "WithWorkDirFS - nil",
			input:       NewSysConfig().WithWorkDirFS(nil),
			expectedErr: "FS for . is nil",
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.input.toSysContext()
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// requireSysContext ensures internalwasm.NewSysContext doesn't return an error, which makes it usable in test matrices.
func requireSysContext(t *testing.T, max uint32, args, environ []string, stdin io.Reader, stdout, stderr io.Writer, openedFiles map[uint32]*internalwasm.FileEntry) *internalwasm.SysContext {
	sys, err := internalwasm.NewSysContext(max, args, environ, stdin, stdout, stderr, openedFiles)
	require.NoError(t, err)
	return sys
}
