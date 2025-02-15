package wazero

import (
	"bytes"
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
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
		{
			name: "WithDebugInfoEnabled",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithDebugInfoEnabled(false)
			},
			expected: &runtimeConfig{
				dwarfDisabled: true, // dwarf is a more technical name and ok here.
			},
		},
		{
			name: "WithCustomSections",
			with: func(c RuntimeConfig) RuntimeConfig {
				return c.WithCustomSections(true)
			},
			expected: &runtimeConfig{
				storeCustomSections: true,
			},
		},
		{
			name:     "WithCloseOnContextDone",
			with:     func(c RuntimeConfig) RuntimeConfig { return c.WithCloseOnContextDone(true) },
			expected: &runtimeConfig{ensureTermination: true},
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
		name          string
		with          func(ModuleConfig) ModuleConfig
		expectNameSet bool
		expectedName  string
	}{
		{
			name: "WithName default",
			with: func(c ModuleConfig) ModuleConfig {
				return c
			},
			expectNameSet: false,
			expectedName:  "",
		},
		{
			name: "WithName",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero")
			},
			expectNameSet: true,
			expectedName:  "wazero",
		},
		{
			name: "WithName empty",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("")
			},
			expectNameSet: true,
			expectedName:  "",
		},
		{
			name: "WithName twice",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero").WithName("wa0")
			},
			expectNameSet: true,
			expectedName:  "wa0",
		},
		{
			name: "WithName can clear",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero").WithName("")
			},
			expectNameSet: true,
			expectedName:  "",
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := NewModuleConfig()
			rc := tc.with(input)
			require.Equal(t, tc.expectNameSet, rc.(*moduleConfig).nameSet)
			require.Equal(t, tc.expectedName, rc.(*moduleConfig).name)
			// The source wasn't modified
			require.Equal(t, NewModuleConfig(), input)
		})
	}
}

// TestModuleConfig_toSysContext only tests the cases that change the inputs to
// sys.NewContext.
func TestModuleConfig_toSysContext(t *testing.T) {
	base := NewModuleConfig()

	tests := []struct {
		name  string
		input func() (mc ModuleConfig, verify func(t *testing.T, sys *internalsys.Context))
	}{
		{
			name: "empty",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				return base, func(t *testing.T, sys *internalsys.Context) { require.NotNil(t, sys) }
			},
		},
		{
			name: "WithNanotime",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithNanotime(func() int64 { return 1234567 }, 54321)
				return config, func(t *testing.T, sys *internalsys.Context) {
					require.Equal(t, 1234567, int(sys.Nanotime()))
					require.Equal(t, 54321, int(sys.NanotimeResolution()))
				}
			},
		},
		{
			name: "WithSysNanotime",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithSysNanotime()
				return config, func(t *testing.T, sys *internalsys.Context) {
					require.Equal(t, int(1), int(sys.NanotimeResolution()))
				}
			},
		},
		{
			name: "WithWalltime",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithWalltime(func() (sec int64, nsec int32) { return 5, 10 }, 54321)
				return config, func(t *testing.T, sys *internalsys.Context) {
					actualSec, actualNano := sys.Walltime()
					require.Equal(t, 5, int(actualSec))
					require.Equal(t, 10, int(actualNano))
					require.Equal(t, 54321, int(sys.WalltimeResolution()))
				}
			},
		},
		{
			name: "WithSysWalltime",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithSysWalltime()
				return config, func(t *testing.T, sys *internalsys.Context) {
					require.Equal(t, int(time.Microsecond.Nanoseconds()), int(sys.WalltimeResolution()))
				}
			},
		},
		{
			name: "WithArgs empty",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithArgs()
				return config, func(t *testing.T, sys *internalsys.Context) {
					args := sys.Args()
					require.Equal(t, 0, len(args))
				}
			},
		},
		{
			name: "WithArgs",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithArgs("a", "bc")
				return config, func(t *testing.T, sys *internalsys.Context) {
					args := sys.Args()
					require.Equal(t, 2, len(args))
					require.Equal(t, "a", string(args[0]))
					require.Equal(t, "bc", string(args[1]))
				}
			},
		},
		{
			name: "WithArgs empty ok", // Particularly argv[0] can be empty, and we have no rules about others.
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithArgs("", "bc")
				return config, func(t *testing.T, sys *internalsys.Context) {
					args := sys.Args()
					require.Equal(t, 2, len(args))
					require.Equal(t, "", string(args[0]))
					require.Equal(t, "bc", string(args[1]))
				}
			},
		},
		{
			name: "WithArgs second call overwrites",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithArgs("a", "bc").WithArgs("bc", "a")
				return config, func(t *testing.T, sys *internalsys.Context) {
					args := sys.Args()
					require.Equal(t, 2, len(args))
					require.Equal(t, "bc", string(args[0]))
					require.Equal(t, "a", string(args[1]))
				}
			},
		},
		{
			name: "WithEnv",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithEnv("a", "b")
				return config, func(t *testing.T, sys *internalsys.Context) {
					envs := sys.Environ()
					require.Equal(t, 1, len(envs))
					require.Equal(t, "a=b", string(envs[0]))
				}
			},
		},
		{
			name: "WithEnv empty value",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithEnv("a", "")
				return config, func(t *testing.T, sys *internalsys.Context) {
					envs := sys.Environ()
					require.Equal(t, 1, len(envs))
					require.Equal(t, "a=", string(envs[0]))
				}
			},
		},
		{
			name: "WithEnv twice",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithEnv("a", "b").WithEnv("c", "de")
				return config, func(t *testing.T, sys *internalsys.Context) {
					envs := sys.Environ()
					require.Equal(t, 2, len(envs))
					require.Equal(t, "a=b", string(envs[0]))
					require.Equal(t, "c=de", string(envs[1]))
				}
			},
		},
		{
			name: "WithEnv overwrites",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithEnv("a", "bc").WithEnv("c", "de").WithEnv("a", "ff")
				return config, func(t *testing.T, sys *internalsys.Context) {
					envs := sys.Environ()
					require.Equal(t, 2, len(envs))
					require.Equal(t, "a=ff", string(envs[0]))
					require.Equal(t, "c=de", string(envs[1]))
				}
			},
		},
		{
			name: "WithEnv twice",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithEnv("a", "b").WithEnv("c", "de")
				return config, func(t *testing.T, sys *internalsys.Context) {
					envs := sys.Environ()
					require.Equal(t, 2, len(envs))
					require.Equal(t, "a=b", string(envs[0]))
					require.Equal(t, "c=de", string(envs[1]))
				}
			},
		},
		{
			name: "WithRandSource",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				r := bytes.NewReader([]byte{1, 2, 3, 4})
				config := base.WithRandSource(r)
				return config, func(t *testing.T, sys *internalsys.Context) {
					actual := sys.RandSource()
					require.Equal(t, r, actual)
				}
			},
		},
		{
			name: "WithRandSource nil",
			input: func() (ModuleConfig, func(t *testing.T, sys *internalsys.Context)) {
				config := base.WithRandSource(nil)
				return config, func(t *testing.T, sys *internalsys.Context) {
					actual := sys.RandSource()
					require.Equal(t, platform.NewFakeRandSource(), actual)
				}
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			config, verify := tc.input()
			actual, err := config.(*moduleConfig).toSysContext()
			require.NoError(t, err)
			verify(t, actual)
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

// TestModuleConfig_toSysContext_WithOsyield has to test differently because
// we can't compare function pointers when functions are passed by value.
func TestModuleConfig_toSysContext_WithOsyield(t *testing.T) {
	var yielded bool
	sysCtx, err := NewModuleConfig().
		WithOsyield(func() {
			yielded = true
		}).(*moduleConfig).toSysContext()
	require.NoError(t, err)
	sysCtx.Osyield()
	require.True(t, yielded)
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
	mc.fsConfig = NewFSConfig().WithFSMount(fstest.FS, "/")
	mc.environKeys["2"] = 2

	cloned.environKeys["1"] = 1

	// Ensure the maps are not shared
	require.Equal(t, map[string]int{"2": 2}, mc.environKeys)
	require.Equal(t, map[string]int{"1": 1}, cloned.environKeys)

	// Ensure the fs is not shared
	require.Nil(t, cloned.fsConfig)
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

func Test_compiledModule_CustomSections(t *testing.T) {
	tests := []struct {
		name     string
		input    *compiledModule
		expected []string
	}{
		{
			name:     "no custom section",
			input:    &compiledModule{module: &wasm.Module{}},
			expected: []string{},
		},
		{
			name: "name",
			input: &compiledModule{module: &wasm.Module{
				CustomSections: []*wasm.CustomSection{
					{Name: "custom1"},
					{Name: "custom2"},
					{Name: "customDup"},
					{Name: "customDup"},
				},
			}},
			expected: []string{
				"custom1",
				"custom2",
				"customDup",
				"customDup",
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			customSections := tc.input.CustomSections()
			require.Equal(t, len(tc.expected), len(customSections))
			for i := 0; i < len(tc.expected); i++ {
				require.Equal(t, tc.expected[i], customSections[i].Name())
			}
		})
	}
}

func Test_compiledModule_Close(t *testing.T) {
	for _, ctx := range []context.Context{nil, testCtx} { // Ensure it doesn't crash on nil!
		e := &mockEngine{name: "1", cachedModules: map[*wasm.Module]struct{}{}}

		var cs []*compiledModule
		for i := 0; i < 10; i++ {
			m := &wasm.Module{}
			err := e.CompileModule(ctx, m, nil, false)
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

func TestNewRuntimeConfig(t *testing.T) {
	c, ok := NewRuntimeConfig().(*runtimeConfig)
	require.True(t, ok)
	// Should be cloned from the source.
	require.NotEqual(t, engineLessConfig, c)
	// Ensures if the correct engine is selected.
	require.Equal(t, engineKindAuto, c.engineKind)
}
