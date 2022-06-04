package wazero

import (
	"context"
	"io"
	"math"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/api"
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
	im := func(externType api.ExternType, oldModule, oldName string) (newModule, newName string) {
		return "a", oldName
	}
	im2 := func(externType api.ExternType, oldModule, oldName string) (newModule, newName string) {
		return "b", oldName
	}
	mp := func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
		return 0, 1, 1
	}
	tests := []struct {
		name     string
		with     func(CompileConfig) CompileConfig
		expected *compileConfig
	}{
		{
			name: "WithImportRenamer",
			with: func(c CompileConfig) CompileConfig {
				return c.WithImportRenamer(im)
			},
			expected: &compileConfig{importRenamer: im},
		},
		{
			name: "WithImportRenamer twice",
			with: func(c CompileConfig) CompileConfig {
				return c.WithImportRenamer(im).WithImportRenamer(im2)
			},
			expected: &compileConfig{importRenamer: im2},
		},
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
			require.Equal(t, reflect.ValueOf(tc.expected.importRenamer), reflect.ValueOf(rc.importRenamer))
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
		expected ModuleConfig
	}{
		{
			name: "WithName",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero")
			},
			expected: &moduleConfig{
				name: "wazero",
			},
		},
		{
			name: "WithName empty",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("")
			},
			expected: &moduleConfig{},
		},
		{
			name: "WithName twice",
			with: func(c ModuleConfig) ModuleConfig {
				return c.WithName("wazero").WithName("wa0")
			},
			expected: &moduleConfig{
				name: "wa0",
			},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			input := &moduleConfig{}
			rc := tc.with(input)
			require.Equal(t, tc.expected, rc)
			// The source wasn't modified
			require.Equal(t, &moduleConfig{}, input)
		})
	}
}

func TestModuleConfig_toSysContext(t *testing.T) {
	testFS := fstest.MapFS{}
	testFS2 := fstest.MapFS{}

	tests := []struct {
		name     string
		input    ModuleConfig
		expected *internalsys.Context
	}{
		{
			name:  "empty",
			input: NewModuleConfig(),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithArgs",
			input: NewModuleConfig().WithArgs("a", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"a", "bc"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // randSource
				nil, 0,              // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithArgs empty ok", // Particularly argv[0] can be empty, and we have no rules about others.
			input: NewModuleConfig().WithArgs("", "bc"),
			expected: requireSysContext(t,
				math.MaxUint32,     // max
				[]string{"", "bc"}, // args
				nil,                // environ
				nil,                // stdin
				nil,                // stdout
				nil,                // stderr
				nil,                // randSource
				nil, 0,             // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithArgs second call overwrites",
			input: NewModuleConfig().WithArgs("a", "bc").WithArgs("bc", "a"),
			expected: requireSysContext(t,
				math.MaxUint32,      // max
				[]string{"bc", "a"}, // args
				nil,                 // environ
				nil,                 // stdin
				nil,                 // stdout
				nil,                 // stderr
				nil,                 // randSource
				nil, 0,              // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithEnv",
			input: NewModuleConfig().WithEnv("a", "b"),
			expected: requireSysContext(t,
				math.MaxUint32,  // max
				nil,             // args
				[]string{"a=b"}, // environ
				nil,             // stdin
				nil,             // stdout
				nil,             // stderr
				nil,             // randSource
				nil, 0,          // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithEnv empty value",
			input: NewModuleConfig().WithEnv("a", ""),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				[]string{"a="}, // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithEnv twice",
			input: NewModuleConfig().WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // randSource
				nil, 0,                  // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithEnv overwrites",
			input: NewModuleConfig().WithEnv("a", "bc").WithEnv("c", "de").WithEnv("a", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,           // max
				nil,                      // args
				[]string{"a=de", "c=de"}, // environ
				nil,                      // stdin
				nil,                      // stdout
				nil,                      // stderr
				nil,                      // randSource
				nil, 0,                   // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},

		{
			name:  "WithEnv twice",
			input: NewModuleConfig().WithEnv("a", "b").WithEnv("c", "de"),
			expected: requireSysContext(t,
				math.MaxUint32,          // max
				nil,                     // args
				[]string{"a=b", "c=de"}, // environ
				nil,                     // stdin
				nil,                     // stdout
				nil,                     // stderr
				nil,                     // randSource
				nil, 0,                  // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			),
		},
		{
			name:  "WithFS",
			input: NewModuleConfig().WithFS(testFS),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				map[uint32]*internalsys.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS},
					4: {Path: ".", FS: testFS},
				},
			),
		},
		{
			name:  "WithFS overwrites",
			input: NewModuleConfig().WithFS(testFS).WithFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				map[uint32]*internalsys.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS2},
					4: {Path: ".", FS: testFS2},
				},
			),
		},
		{
			name:  "WithWorkDirFS",
			input: NewModuleConfig().WithWorkDirFS(testFS),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				map[uint32]*internalsys.FileEntry{ // openedFiles
					3: {Path: ".", FS: testFS},
				},
			),
		},
		{
			name:  "WithFS and WithWorkDirFS",
			input: NewModuleConfig().WithFS(testFS).WithWorkDirFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				map[uint32]*internalsys.FileEntry{ // openedFiles
					3: {Path: "/", FS: testFS},
					4: {Path: ".", FS: testFS2},
				},
			),
		},
		{
			name:  "WithWorkDirFS and WithFS",
			input: NewModuleConfig().WithWorkDirFS(testFS).WithFS(testFS2),
			expected: requireSysContext(t,
				math.MaxUint32, // max
				nil,            // args
				nil,            // environ
				nil,            // stdin
				nil,            // stdout
				nil,            // stderr
				nil,            // randSource
				nil, 0,         // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				map[uint32]*internalsys.FileEntry{ // openedFiles
					3: {Path: ".", FS: testFS},
					4: {Path: "/", FS: testFS2},
				},
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
		{
			name:        "WithFS nil",
			input:       NewModuleConfig().WithFS(nil),
			expectedErr: "FS for / is nil",
		},
		{
			name:        "WithWorkDirFS nil",
			input:       NewModuleConfig().WithWorkDirFS(nil),
			expectedErr: "FS for . is nil",
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
	openedFiles map[uint32]*internalsys.FileEntry,
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
		openedFiles,
	)
	require.NoError(t, err)
	return sysCtx
}

func TestCompiledCode_Close(t *testing.T) {
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
