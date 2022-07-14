package wazero

import (
	"context"
	"io"
	"io/fs"
	"math"
	"reflect"
	"testing"
	"testing/fstest"

	internalsys "github.com/tetratelabs/wazero/internal/sys"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

func TestRuntimeConfig(t *testing.T) {
	tests := []struct {
		name     string
		with     func(RuntimeConfig) RuntimeConfig
		expected RuntimeConfig
	}{
		{
			name: "bulk-memory-operations",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureBulkMemoryOperations(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureBulkMemoryOperations | wasm.FeatureReferenceTypes,
			},
		},
		{
			name: "multi-value",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureMultiValue(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureMultiValue,
			},
		},
		{
			name: "mutable-global",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureMutableGlobal(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureMutableGlobal,
			},
		},
		{
			name: "nontrapping-float-to-int-conversion",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureNonTrappingFloatToIntConversion(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureNonTrappingFloatToIntConversion,
			},
		},
		{
			name: "sign-extension-ops",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureSignExtensionOps(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureSignExtensionOps,
			},
		},
		{
			name: "REC-wasm-core-1-20191205",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureSignExtensionOps(true).WithWasmCore1()
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.Features20191205,
			},
		},
		{
			name: "WD-wasm-core-2-20220419",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureMutableGlobal(false).WithWasmCore2()
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.Features20220419,
			},
		},
		{
			name: "reference-types",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureReferenceTypes(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureBulkMemoryOperations | wasm.FeatureReferenceTypes,
			},
		},
		{
			name: "simd",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithFeatureSIMD(true)
			},
			expected: &runtimeConfig{
				enabledFeatures: wasm.FeatureSIMD,
			},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := &runtimeConfig{}
			rc := tc.with(input)
			require.Equal(t, tc.expected, rc)
			// The source wasn't modified
			require.Equal(t, &runtimeConfig{}, input)
		})
	}
}

func TestRuntimeConfig_FeatureToggle(t *testing.T) {
	tests := []struct {
		name          string
		feature       wasm.Features
		expectDefault bool
		setFeature    func(RuntimeConfig, bool) RuntimeConfig
	}{
		{
			name:          "bulk-memory-operations",
			feature:       wasm.FeatureBulkMemoryOperations,
			expectDefault: false,
			setFeature: func(c RuntimeConfig, v bool) RuntimeConfig {
				return c.WithFeatureBulkMemoryOperations(v)
			},
		},
		{
			name:          "multi-value",
			feature:       wasm.FeatureMultiValue,
			expectDefault: false,
			setFeature: func(c RuntimeConfig, v bool) RuntimeConfig {
				return c.WithFeatureMultiValue(v)
			},
		},
		{
			name:          "mutable-global",
			feature:       wasm.FeatureMutableGlobal,
			expectDefault: true,
			setFeature: func(c RuntimeConfig, v bool) RuntimeConfig {
				return c.WithFeatureMutableGlobal(v)
			},
		},
		{
			name:          "nontrapping-float-to-int-conversion",
			feature:       wasm.FeatureNonTrappingFloatToIntConversion,
			expectDefault: false,
			setFeature: func(c RuntimeConfig, v bool) RuntimeConfig {
				return c.WithFeatureNonTrappingFloatToIntConversion(v)
			},
		},
		{
			name:          "sign-extension-ops",
			feature:       wasm.FeatureSignExtensionOps,
			expectDefault: false,
			setFeature: func(c RuntimeConfig, v bool) RuntimeConfig {
				return c.WithFeatureSignExtensionOps(v)
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			c := NewRuntimeConfig().(*runtimeConfig)
			require.Equal(t, tc.expectDefault, c.enabledFeatures.Get(tc.feature))

			// Set to false even if it was initially false.
			c = tc.setFeature(c, false).(*runtimeConfig)
			require.False(t, c.enabledFeatures.Get(tc.feature))

			// Set true makes it true
			c = tc.setFeature(c, true).(*runtimeConfig)
			require.True(t, c.enabledFeatures.Get(tc.feature))

			// Set false makes it false again
			c = tc.setFeature(c, false).(*runtimeConfig)
			require.False(t, c.enabledFeatures.Get(tc.feature))
		})
	}
}

func TestCompileConfig(t *testing.T) {
	mp := func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
		return 0, 1, 1
	}
	tests := []struct {
		name     string
		with     func(CompileConfig) CompileConfig
		expected *compileConfig
	}{
		{
			name: "WithMemorySizer",
			with: func(c CompileConfig) CompileConfig {
				return c.WithMemorySizer(mp)
			},
			expected: &compileConfig{memorySizer: mp},
		},
		{
			name: "WithMemorySizer twice",
			with: func(c CompileConfig) CompileConfig {
				return c.WithMemorySizer(wasm.MemorySizer).WithMemorySizer(mp)
			},
			expected: &compileConfig{memorySizer: mp},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := &compileConfig{}
			rc := tc.with(input).(*compileConfig)

			// We cannot compare func, but we can compare reflect.Value
			// See https://go.dev/ref/spec#Comparison_operators
			require.Equal(t, reflect.ValueOf(tc.expected.memorySizer), reflect.ValueOf(rc.memorySizer))
			// The source wasn't modified
			require.Equal(t, &compileConfig{}, input)
		})
	}
}

func TestModuleConfig(t *testing.T) {
	tests := []struct {
		name     string
		with     func(ModuleConfig) ModuleConfig
		expected string
	}{
		{
			name: "WithName",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero")
			},
			expected: "wazero",
		},
		{
			name: "WithName empty",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("")
			},
		},
		{
			name: "WithName twice",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero").WithName("wa0")
			},
			expected: "wa0",
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := NewModuleConfig()
			rc := tc.with(input)
			require.Equal(t, tc.expected, rc.(*moduleConfig).name)
			// The source wasn't modified
			require.Equal(t, NewModuleConfig(), input)
		})
	}
}

// TestModuleConfig_toSysContext only tests the cases that change the inputs to
// sys.NewContext.
func TestModuleConfig_toSysContext(t *testing.T) {
	// Always assigns clocks so that pointers are constant.
	var wt sys.Walltime = func(context.Context) (int64, int32) {
		return 0, 0
	}
	var nt sys.Nanotime = func(context.Context) int64 {
		return 0
	}
	base := NewModuleConfig()
	base.(*moduleConfig).walltime = &wt
	base.(*moduleConfig).walltimeResolution = 1
	base.(*moduleConfig).nanotime = &nt
	base.(*moduleConfig).nanotimeResolution = 1

	testFS := testfs.FS{}
	testFS2 := testfs.FS{"/": &testfs.File{}}

	tests := []struct {
		name     string
		input    ModuleConfig
		expected *internalsys.Context
	}{
		{
			name:  "empty",
			input: base,
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				&wt, 1,         // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithArgs",
			input: base.WithArgs("a", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"a", "bc"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // randSource
				&wt, 1,              // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithArgs empty ok", // Particularly argv[0] can be empty, and we have no rules about others.
			input: base.WithArgs("", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,     // max
				[]string{"", "bc"}, // args
				nil,                // environ
				nil,                // stdin
				nil,                // stdout
				nil,                // stderr
				nil,                // randSource
				&wt, 1,             // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithArgs second call overwrites",
			input: base.WithArgs("a", "bc").WithArgs("bc", "a"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"bc", "a"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // randSource
				&wt, 1,              // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithEnv",
			input: base.WithEnv("a", "b"),
			expected: requireSysContext(t,
				math.MaxUint32,  // max
				nil,             // args
				[]string{"a=b"}, // environ
				nil,             // stdin
				nil,             // stdout
				nil,             // stderr
				nil,             // randSource
				&wt, 1,          // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithEnv empty value",
			input: base.WithEnv("a", ""),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				[]string{"a="}, // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				&wt, 1,         // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithEnv twice",
			input: base.WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // randSource
				&wt, 1,                  // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithEnv overwrites",
			input: base.WithEnv("a", "bc").WithEnv("c", "de").WithEnv("a", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,           // max
				nil,                      // args
				[]string{"a=de", "c=de"}, // environ
				nil,                      // stdin
				nil,                      // stdout
				nil,                      // stderr
				nil,                      // randSource
				&wt, 1,                   // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithEnv twice",
			input: base.WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // randSource
				&wt, 1,                  // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			),
		},
		{
			name:  "WithFS",
			input: base.WithFS(testFS),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				&wt, 1,         // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				testFS,
			),
		},
		{
			name:  "WithFS overwrites",
			input: base.WithFS(testFS).WithFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				&wt, 1,         // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil,     // nanosleep
				testFS2, // fs
			),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := tc.input.(*moduleConfig).toSysContext()
			require.NoError(t, err)
			require.Equal(t, tc.expected, sysCtx)
		})
	}
}

// TestModuleConfig_toSysContext_WithWalltime has to test differently because we can't
// compare function pointers when functions are passed by value.
func TestModuleConfig_toSysContext_WithWalltime(t *testing.T) {
	tests := []struct {
		name               string
		input              ModuleConfig
		expectedSec        int64
		expectedNsec       int32
		expectedResolution sys.ClockResolution
		expectedErr        string
	}{
		{
			name: "ok",
			input: NewModuleConfig().
				WithWalltime(func(context.Context) (sec int64, nsec int32) {
					return 1, 2
				}, 3),
			expectedSec:        1,
			expectedNsec:       2,
			expectedResolution: 3,
		},
		{
			name: "overwrites",
			input: NewModuleConfig().
				WithWalltime(func(context.Context) (sec int64, nsec int32) {
					return 3, 4
				}, 5).
				WithWalltime(func(context.Context) (sec int64, nsec int32) {
					return 1, 2
				}, 3),
			expectedSec:        1,
			expectedNsec:       2,
			expectedResolution: 3,
		},
		{
			name: "invalid resolution",
			input: NewModuleConfig().
				WithWalltime(func(context.Context) (sec int64, nsec int32) {
					return 1, 2
				}, 0),
			expectedErr: "invalid Walltime resolution: 0",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := tc.input.(*moduleConfig).toSysContext()
			if tc.expectedErr == "" {
				require.Nil(t, err)
				sec, nsec := sysCtx.Walltime(testCtx)
				require.Equal(t, tc.expectedSec, sec)
				require.Equal(t, tc.expectedNsec, nsec)
				require.Equal(t, tc.expectedResolution, sysCtx.WalltimeResolution())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}

	t.Run("context", func(t *testing.T) {
		sysCtx, err := NewModuleConfig().
			WithWalltime(func(ctx context.Context) (sec int64, nsec int32) {
				require.Equal(t, testCtx, ctx)
				return 1, 2
			}, 3).(*moduleConfig).toSysContext()
		require.NoError(t, err)
		sec, nsec := sysCtx.Walltime(testCtx)
		// If below pass, the context was correct!
		require.Equal(t, int64(1), sec)
		require.Equal(t, int32(2), nsec)
	})
}

// TestModuleConfig_toSysContext_WithNanotime has to test differently because we can't
// compare function pointers when functions are passed by value.
func TestModuleConfig_toSysContext_WithNanotime(t *testing.T) {
	tests := []struct {
		name               string
		input              ModuleConfig
		expectedNanos      int64
		expectedResolution sys.ClockResolution
		expectedErr        string
	}{
		{
			name: "ok",
			input: NewModuleConfig().
				WithNanotime(func(context.Context) int64 {
					return 1
				}, 2),
			expectedNanos:      1,
			expectedResolution: 2,
		},
		{
			name: "overwrites",
			input: NewModuleConfig().
				WithNanotime(func(context.Context) int64 {
					return 3
				}, 4).
				WithNanotime(func(context.Context) int64 {
					return 1
				}, 2),
			expectedNanos:      1,
			expectedResolution: 2,
		},
		{
			name: "invalid resolution",
			input: NewModuleConfig().
				WithNanotime(func(context.Context) int64 {
					return 1
				}, 0),
			expectedErr: "invalid Nanotime resolution: 0",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := tc.input.(*moduleConfig).toSysContext()
			if tc.expectedErr == "" {
				require.Nil(t, err)
				nanos := sysCtx.Nanotime(testCtx)
				require.Equal(t, tc.expectedNanos, nanos)
				require.Equal(t, tc.expectedResolution, sysCtx.NanotimeResolution())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}

	t.Run("context", func(t *testing.T) {
		sysCtx, err := NewModuleConfig().
			WithNanotime(func(ctx context.Context) int64 {
				require.Equal(t, testCtx, ctx)
				return 1
			}, 2).(*moduleConfig).toSysContext()
		require.NoError(t, err)
		// If below pass, the context was correct!
		require.Equal(t, int64(1), sysCtx.Nanotime(testCtx))
	})
}

// TestModuleConfig_toSysContext_WithNanosleep has to test differently because
// we can't compare function pointers when functions are passed by value.
func TestModuleConfig_toSysContext_WithNanosleep(t *testing.T) {
	sysCtx, err := NewModuleConfig().
		WithNanosleep(func(ctx context.Context, ns int64) {
			require.Equal(t, testCtx, ctx)
		}).(*moduleConfig).toSysContext()
	require.NoError(t, err)
	// If below pass, the context was correct!
	sysCtx.Nanosleep(testCtx, 2)
}

func TestModuleConfig_toSysContext_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       ModuleConfig
		expectedErr string
	}{
		{
			name:        "WithArgs arg contains NUL",
			input:       NewModuleConfig().WithArgs("", string([]byte{'a', 0})),
			expectedErr: "args invalid: contains NUL character",
		},
		{
			name:        "WithEnv key contains NUL",
			input:       NewModuleConfig().WithEnv(string([]byte{'a', 0}), "a"),
			expectedErr: "environ invalid: contains NUL character",
		},
		{
			name:        "WithEnv value contains NUL",
			input:       NewModuleConfig().WithEnv("a", string([]byte{'a', 0})),
			expectedErr: "environ invalid: contains NUL character",
		},
		{
			name:        "WithEnv key contains equals",
			input:       NewModuleConfig().WithEnv("a=", "a"),
			expectedErr: "environ invalid: key contains '=' character",
		},
		{
			name:        "WithEnv empty key",
			input:       NewModuleConfig().WithEnv("", "a"),
			expectedErr: "environ invalid: empty key",
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.input.(*moduleConfig).toSysContext()
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestModuleConfig_clone(t *testing.T) {
	mc := NewModuleConfig().(*moduleConfig)
	cloned := mc.clone()

	// Make post-clone changes
	mc.fs = fstest.MapFS{}
	mc.environKeys["2"] = 2

	cloned.environKeys["1"] = 1

	// Ensure the maps are not shared
	require.Equal(t, map[string]int{"2": 2}, mc.environKeys)
	require.Equal(t, map[string]int{"1": 1}, cloned.environKeys)

	// Ensure the fs is not shared
	require.Nil(t, cloned.fs)
}

func Test_compiledModule_Name(t *testing.T) {
	tests := []struct {
		name     string
		input    *compiledModule
		expected string
	}{
		{
			name:  "no name section",
			input: &compiledModule{module: &wasm.Module{}},
		},
		{
			name:  "empty name",
			input: &compiledModule{module: &wasm.Module{NameSection: &wasm.NameSection{}}},
		},
		{
			name:     "name",
			input:    &compiledModule{module: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "foo"}}},
			expected: "foo",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.input.Name())
		})
	}
}

func Test_compiledModule_Close(t *testing.T) {
	for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
		e := &mockEngine{name: "1", cachedModules: map[*wasm.Module]struct{}{}}

		var cs []*compiledModule
		for i := 0; i < 10; i++ {
			m := &wasm.Module{}
			err := e.CompileModule(ctx, m)
			require.NoError(t, err)
			cs = append(cs, &compiledModule{module: m, compiledEngine: e})
		}

		// Before Close.
		require.Equal(t, 10, len(e.cachedModules))

		for _, c := range cs {
			require.NoError(t, c.Close(ctx))
		}

		// After Close.
		require.Zero(t, len(e.cachedModules))
	}
}

// requireSysContext ensures wasm.NewContext doesn't return an error, which makes it usable in test matrices.
func requireSysContext(
	t *testing.T,
	max uint32,
	args, environ []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	randSource io.Reader,
	walltime *sys.Walltime, walltimeResolution sys.ClockResolution,
	nanotime *sys.Nanotime, nanotimeResolution sys.ClockResolution,
	nanosleep *sys.Nanosleep,
	fs fs.FS,
) *internalsys.Context {
	sysCtx, err := internalsys.NewContext(
		max,
		args,
		environ,
		stdin,
		stdout,
		stderr,
		randSource,
		walltime, walltimeResolution,
		nanotime, nanotimeResolution,
		nanosleep,
		fs,
	)
	require.NoError(t, err)
	return sysCtx
}
