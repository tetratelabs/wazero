package wazero

import (
	"context"
	_ "embed"
	"errors"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/watzero"
	"github.com/tetratelabs/wazero/sys"
)

var (
	binaryNamedZero = binaryformat.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "0"}})
	// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
	testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")
	zero    = wasm.Index(0)
)

func TestRuntime_CompileModule(t *testing.T) {
	tests := []struct {
		name         string
		runtime      Runtime
		wasm         []byte
		expectedName string
	}{
		{
			name: "no name section",
			wasm: binaryformat.EncodeModule(&wasm.Module{}),
		},
		{
			name: "empty NameSection.ModuleName",
			wasm: binaryformat.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{}}),
		},
		{
			name:         "NameSection.ModuleName",
			wasm:         binaryformat.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "test"}}),
			expectedName: "test",
		},
	}

	r := NewRuntime()
	defer r.Close(testCtx)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := r.CompileModule(testCtx, tc.wasm, NewCompileConfig())
			require.NoError(t, err)
			code := m.(*compiledModule)
			if tc.expectedName != "" {
				require.Equal(t, tc.expectedName, code.module.NameSection.ModuleName)
			}
			require.Equal(t, r.(*runtime).store.Engine, code.compiledEngine)
		})
	}

	t.Run("WithMemorySizer", func(t *testing.T) {
		testWasm := binaryformat.EncodeModule(&wasm.Module{MemorySection: &wasm.Memory{Min: 1}})

		m, err := r.CompileModule(testCtx, testWasm, NewCompileConfig().
			WithMemorySizer(func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
				return 1, 2, 3
			}))
		require.NoError(t, err)
		code := m.(*compiledModule)

		require.Equal(t, &wasm.Memory{
			Min: 1,
			Cap: 2,
			Max: 3,
		}, code.module.MemorySection)
	})
}

func TestRuntime_CompileModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		config      CompileConfig
		wasm        []byte
		expectedErr string
	}{
		{
			name:        "nil",
			expectedErr: "binary == nil",
		},
		{
			name:        "invalid binary",
			wasm:        append(binaryformat.Magic, []byte("yolo")...),
			expectedErr: "invalid version header",
		},
		{
			name: "memory cap < min", // only one test to avoid duplicating tests in module_test.go
			config: NewCompileConfig().WithMemorySizer(func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
				return 3, 1, 3
			}),
			wasm: binaryformat.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{Min: 3},
			}),
			expectedErr: "section memory: capacity 1 pages (64 Ki) less than minimum 3 pages (192 Ki)",
		},
		{
			name: "memory cap < min exported", // only one test to avoid duplicating tests in module_test.go
			config: NewCompileConfig().WithMemorySizer(func(minPages uint32, maxPages *uint32) (min, capacity, max uint32) {
				return 3, 2, 3
			}),
			wasm: binaryformat.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{},
				ExportSection: []*wasm.Export{
					{Name: "memory", Type: api.ExternTypeMemory},
				},
			}),
			expectedErr: "section memory: capacity 2 pages (128 Ki) less than minimum 3 pages (192 Ki)",
		},
		{
			name:        "memory has too many pages",
			wasm:        binaryformat.EncodeModule(&wasm.Module{MemorySection: &wasm.Memory{Min: 2, Cap: 2, Max: 70000, IsMaxEncoded: true}}),
			expectedErr: "section memory: max 70000 pages (4 Gi) over limit of 65536 pages (4 Gi)",
		},
	}

	r := NewRuntime()
	defer r.Close(testCtx)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			config := tc.config
			if config == nil {
				config = NewCompileConfig()
			}
			_, err := r.CompileModule(testCtx, tc.wasm, config)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// TestModule_Memory only covers a couple cases to avoid duplication of internal/wasm/runtime_test.go
func TestModule_Memory(t *testing.T) {
	tests := []struct {
		name        string
		builder     func(Runtime) ModuleBuilder
		expected    bool
		expectedLen uint32
	}{
		{
			name: "no memory",
			builder: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder(t.Name())
			},
		},
		{
			name: "memory exported, one page",
			builder: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder(t.Name()).ExportMemory("memory", 1)
			},
			expected:    true,
			expectedLen: 65536,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime()
			defer r.Close(testCtx)

			// Instantiate the module and get the export of the above memory
			module, err := tc.builder(r).Instantiate(testCtx, r)
			require.NoError(t, err)

			mem := module.ExportedMemory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size(testCtx))
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

// TestModule_Global only covers a couple cases to avoid duplication of internal/wasm/global_test.go
func TestModule_Global(t *testing.T) {
	globalVal := int64(100) // intentionally a value that differs in signed vs unsigned encoding

	tests := []struct {
		name                      string
		module                    *wasm.Module // module as wat doesn't yet support globals
		builder                   func(Runtime) ModuleBuilder
		expected, expectedMutable bool
	}{
		{
			name:   "no global",
			module: &wasm.Module{},
		},
		{
			name: "global not exported",
			module: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
			},
		},
		{
			name: "global exported",
			builder: func(r Runtime) ModuleBuilder {
				return r.NewModuleBuilder(t.Name()).ExportGlobalI64("global", globalVal)
			},
			expected: true,
		},
		{
			name: "global exported and mutable",
			module: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
				ExportSection: []*wasm.Export{
					{Type: wasm.ExternTypeGlobal, Name: "global"},
				},
			},
			expected:        true,
			expectedMutable: true,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime().(*runtime)
			defer r.Close(testCtx)

			var m CompiledModule
			if tc.module != nil {
				m = &compiledModule{module: tc.module}
			} else {
				m, _ = tc.builder(r).Compile(testCtx, NewCompileConfig())
			}
			code := m.(*compiledModule)

			err := r.store.Engine.CompileModule(testCtx, code.module)
			require.NoError(t, err)

			// Instantiate the module and get the export of the above global
			module, err := r.InstantiateModule(testCtx, code, NewModuleConfig())
			require.NoError(t, err)

			global := module.ExportedGlobal("global")
			if !tc.expected {
				require.Nil(t, global)
				return
			}
			require.Equal(t, uint64(globalVal), global.Get(testCtx))

			mutable, ok := global.(api.MutableGlobal)
			require.Equal(t, tc.expectedMutable, ok)
			if ok {
				mutable.Set(testCtx, 2)
				require.Equal(t, uint64(2), global.Get(testCtx))
			}
		})
	}
}

func TestModule_FunctionContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected context.Context
	}{
		{
			name:     "nil defaults to context.Background",
			ctx:      nil,
			expected: context.Background(),
		},
		{
			name:     "set context",
			ctx:      testCtx,
			expected: testCtx,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime()
			defer r.Close(testCtx)

			// Define a host function so that we can catch the context propagated from a module function call
			functionName := "fn"
			expectedResult := uint64(math.MaxUint64)
			hostFn := func(ctx context.Context) uint64 {
				require.Equal(t, tc.expected, ctx)
				return expectedResult
			}
			source := requireImportAndExportFunction(t, r, hostFn, functionName)

			// Instantiate the module and get the export of the above hostFn
			module, err := r.InstantiateModuleFromBinary(tc.ctx, source)
			require.NoError(t, err)

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := module.ExportedFunction(functionName).Call(tc.ctx)
			require.NoError(t, err)
			require.Equal(t, expectedResult, results[0])
		})
	}
}

func TestRuntime_InstantiateModule_UsesContext(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	// Define a function that will be set as the start function
	var calledStart bool
	start := func(ctx context.Context) {
		calledStart = true
		require.Equal(t, testCtx, ctx)
	}

	_, err := r.NewModuleBuilder("env").
		ExportFunction("start", start).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	binary := binaryformat.EncodeModule(&wasm.Module{
		TypeSection:   []*wasm.FunctionType{{}},
		ImportSection: []*wasm.Import{{Module: "env", Name: "start", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		StartSection:  &zero,
	})

	code, err := r.CompileModule(testCtx, binary, NewCompileConfig())
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	mod, err := r.InstantiateModule(testCtx, code, NewModuleConfig())
	require.NoError(t, err)

	require.True(t, calledStart)

	// Closing the module shouldn't remove the compiler cache
	require.NoError(t, mod.Close(testCtx))
	require.Equal(t, uint32(2), r.(*runtime).store.Engine.CompiledModuleCount())
}

// TestRuntime_InstantiateModuleFromBinary_DoesntEnforce_Start ensures wapc-go work when modules import WASI, but don't
// export "_start".
func TestRuntime_InstantiateModuleFromBinary_DoesntEnforce_Start(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	binary := binaryformat.EncodeModule(&wasm.Module{
		MemorySection: &wasm.Memory{Min: 1},
		ExportSection: []*wasm.Export{{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0}},
	})

	mod, err := r.InstantiateModuleFromBinary(testCtx, binary)
	require.NoError(t, err)
	require.NoError(t, mod.Close(testCtx))
}

func TestRuntime_InstantiateModuleFromBinary_UsesContext(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	// Define a function that will be re-exported as the WASI function: _start
	var calledStart bool
	start := func(ctx context.Context) {
		calledStart = true
		require.Equal(t, testCtx, ctx)
	}

	host, err := r.NewModuleBuilder("").
		ExportFunction("start", start).
		Instantiate(testCtx, r)
	require.NoError(t, err)
	defer host.Close(testCtx)

	// Start the module as a WASI command. This will fail if the context wasn't as intended.
	startWasm, err := watzero.Wat2Wasm(`(module $start
	(import "" "start" (func $start))
	(memory 1)
	(export "_start" (func $start))
	(export "memory" (memory 0))
)`)
	require.NoError(t, err)

	_, err = r.InstantiateModuleFromBinary(testCtx, startWasm)
	require.NoError(t, err)

	require.True(t, calledStart)
}

func TestRuntime_InstantiateModuleFromBinary_ErrorOnStart(t *testing.T) {
	tests := []struct {
		name, wasm string
	}{
		{
			name: "_start function",
			wasm: `(module
	(import "" "start" (func $start))
	(export "_start" (func $start))
)`,
		},
		{
			name: ".start function",
			wasm: `(module
	(import "" "start" (func $start))
	(start $start)
)`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime()
			defer r.Close(testCtx)

			start := func(context.Context) {
				panic(errors.New("ice cream"))
			}

			host, err := r.NewModuleBuilder("").
				ExportFunction("start", start).
				Instantiate(testCtx, r)
			require.NoError(t, err)

			// Start the module as a WASI command. We expect it to fail.
			_, err = r.InstantiateModuleFromBinary(testCtx, []byte(tc.wasm))
			require.Error(t, err)

			// Close the imported module, which should remove its compiler cache.
			require.NoError(t, host.Close(testCtx))

			// The compiler cache of the importing module should be removed on error.
			require.Zero(t, r.(*runtime).store.Engine.CompiledModuleCount())
		})
	}
}

// TestRuntime_InstantiateModule_WithName tests that we can pre-validate (cache) a module and instantiate it under
// different names. This pattern is used in wapc-go.
func TestRuntime_InstantiateModule_WithName(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	base, err := r.CompileModule(testCtx, binaryNamedZero, NewCompileConfig())
	require.NoError(t, err)

	require.Equal(t, "0", base.(*compiledModule).module.NameSection.ModuleName)

	// Use the same runtime to instantiate multiple modules
	internal := r.(*runtime).ns
	m1, err := r.InstantiateModule(testCtx, base, NewModuleConfig().WithName("1"))
	require.NoError(t, err)

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("1"), m1)

	m2, err := r.InstantiateModule(testCtx, base, NewModuleConfig().WithName("2"))
	require.NoError(t, err)

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("2"), m2)
}

func TestRuntime_InstantiateModule_ExitError(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	start := func(ctx context.Context, m api.Module) {
		require.NoError(t, m.CloseWithExitCode(ctx, 2))
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("_start", start).Instantiate(testCtx, r)

	// Ensure the exit error propagated and didn't wrap.
	require.Equal(t, err, sys.NewExitError("env", 2))

	// The compiler cache of the importing module should be removed on error.
	require.Zero(t, r.(*runtime).store.Engine.CompiledModuleCount())
}

func TestRuntime_CloseWithExitCode(t *testing.T) {
	tests := []struct {
		name     string
		exitCode uint32
	}{
		{
			name:     "exit code 0",
			exitCode: uint32(0),
		},
		{
			name:     "exit code 2",
			exitCode: uint32(2),
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime()

			m1, err := r.NewModuleBuilder("mod1").ExportFunction("func1", func() {}).Instantiate(testCtx, r)
			require.NoError(t, err)
			m2, err := r.NewModuleBuilder("mod2").ExportFunction("func2", func() {}).Instantiate(testCtx, r)
			require.NoError(t, err)

			func1 := m1.ExportedFunction("func1")
			func2 := m2.ExportedFunction("func2")

			// Modules not closed so calls succeed

			_, err = func1.Call(testCtx)
			require.NoError(t, err)

			_, err = func2.Call(testCtx)
			require.NoError(t, err)

			if tc.exitCode == 0 {
				err = r.Close(testCtx)
			} else {
				err = r.CloseWithExitCode(testCtx, tc.exitCode)
			}
			require.NoError(t, err)

			// Modules closed so calls fail
			_, err = func1.Call(testCtx)
			require.ErrorIs(t, err, sys.NewExitError("mod1", tc.exitCode))

			_, err = func2.Call(testCtx)
			require.ErrorIs(t, err, sys.NewExitError("mod2", tc.exitCode))
		})
	}
}

func TestRuntime_Close_ClosesCompiledModules(t *testing.T) {
	engine := &mockEngine{name: "mock", cachedModules: map[*wasm.Module]struct{}{}}
	conf := *engineLessConfig
	conf.newEngine = func(wasm.Features) wasm.Engine {
		return engine
	}
	r := NewRuntimeWithConfig(&conf)
	defer r.Close(testCtx)

	// Normally compiled modules are closed when instantiated but this is never instantiated.
	_, err := r.CompileModule(testCtx, binaryNamedZero, NewCompileConfig())
	require.NoError(t, err)
	require.Equal(t, uint32(1), engine.CompiledModuleCount())

	err = r.Close(testCtx)
	require.NoError(t, err)

	// Closing the runtime should remove the compiler cache
	require.Zero(t, engine.CompiledModuleCount())
}

// requireImportAndExportFunction re-exports a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, r Runtime, hostFn func(ctx context.Context) uint64, functionName string) []byte {
	_, err := r.NewModuleBuilder("host").ExportFunction(functionName, hostFn).Instantiate(testCtx, r)
	require.NoError(t, err)

	return binaryformat.EncodeModule(&wasm.Module{
		TypeSection:   []*wasm.FunctionType{{Results: []wasm.ValueType{wasm.ValueTypeI64}}},
		ImportSection: []*wasm.Import{{Module: "host", Name: functionName, Type: wasm.ExternTypeFunc, DescFunc: 0}},
		ExportSection: []*wasm.Export{{Name: functionName, Type: wasm.ExternTypeFunc, Index: 0}},
	})
}

type mockEngine struct {
	name          string
	cachedModules map[*wasm.Module]struct{}
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompileModule(_ context.Context, module *wasm.Module) error {
	e.cachedModules[module] = struct{}{}
	return nil
}

// CompiledModuleCount implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompiledModuleCount() uint32 {
	return uint32(len(e.cachedModules))
}

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) DeleteCompiledModule(module *wasm.Module) {
	delete(e.cachedModules, module)
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *mockEngine) NewModuleEngine(_ string, _ *wasm.Module, _, _ []*wasm.FunctionInstance, _ []*wasm.TableInstance, _ []wasm.TableInitEntry) (wasm.ModuleEngine, error) {
	return nil, nil
}
