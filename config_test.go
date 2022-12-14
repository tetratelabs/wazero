package wazero

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"math"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/api"
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
			name: "features",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithCoreFeatures(api.CoreFeaturesV1)
			},
			expected: &runtimeConfig{
				enabledFeatures: api.CoreFeaturesV1,
			},
		},
		{
			name: "memoryLimitPages",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithMemoryLimitPages(10)
			},
			expected: &runtimeConfig{
				memoryLimitPages: 10,
			},
		},
		{
			name: "memoryCapacityFromMax",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithMemoryCapacityFromMax(true)
			},
			expected: &runtimeConfig{
				memoryCapacityFromMax: true,
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

	t.Run("memoryLimitPages invalid panics", func(t *testing.T) {
		err := require.CapturePanic(func() {
			input := &runtimeConfig{}
			input.WithMemoryLimitPages(wasm.MemoryLimitPages + 1)
		})
		require.EqualError(t, err, "memoryLimitPages invalid: 65537 > 65536")
	})
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
	var wt sys.Walltime = func() (int64, int32) {
		return 0, 0
	}
	var nt sys.Nanotime = func() int64 {
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
		{
			name:  "WithRandSource",
			input: base.WithRandSource(rand.Reader),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				rand.Reader,    // randSource
				&wt, 1,         // walltime, walltimeResolution
				&nt, 1, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
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
				WithWalltime(func() (sec int64, nsec int32) {
					return 1, 2
				}, 3),
			expectedSec:        1,
			expectedNsec:       2,
			expectedResolution: 3,
		},
		{
			name: "overwrites",
			input: NewModuleConfig().
				WithWalltime(func() (sec int64, nsec int32) {
					return 3, 4
				}, 5).
				WithWalltime(func() (sec int64, nsec int32) {
					return 1, 2
				}, 3),
			expectedSec:        1,
			expectedNsec:       2,
			expectedResolution: 3,
		},
		{
			name: "invalid resolution",
			input: NewModuleConfig().
				WithWalltime(func() (sec int64, nsec int32) {
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
				sec, nsec := sysCtx.Walltime()
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
			WithWalltime(func() (sec int64, nsec int32) {
				return 1, 2
			}, 3).(*moduleConfig).toSysContext()
		require.NoError(t, err)
		sec, nsec := sysCtx.Walltime()
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
				WithNanotime(func() int64 {
					return 1
				}, 2),
			expectedNanos:      1,
			expectedResolution: 2,
		},
		{
			name: "overwrites",
			input: NewModuleConfig().
				WithNanotime(func() int64 {
					return 3
				}, 4).
				WithNanotime(func() int64 {
					return 1
				}, 2),
			expectedNanos:      1,
			expectedResolution: 2,
		},
		{
			name: "invalid resolution",
			input: NewModuleConfig().
				WithNanotime(func() int64 {
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
				nanos := sysCtx.Nanotime()
				require.Equal(t, tc.expectedNanos, nanos)
				require.Equal(t, tc.expectedResolution, sysCtx.NanotimeResolution())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

// TestModuleConfig_toSysContext_WithNanosleep has to test differently because
// we can't compare function pointers when functions are passed by value.
func TestModuleConfig_toSysContext_WithNanosleep(t *testing.T) {
	sysCtx, err := NewModuleConfig().
		WithNanosleep(func(ns int64) {
			require.Equal(t, int64(2), ns)
		}).(*moduleConfig).toSysContext()
	require.NoError(t, err)
	sysCtx.Nanosleep(2)
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
			err := e.CompileModule(ctx, m, nil)
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
		toByteSlices(args),
		toByteSlices(environ),
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
