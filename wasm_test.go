package wazero

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/sys"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func TestRuntime_CompileModule(t *testing.T) {
	tests := []struct {
		name         string
		runtime      Runtime
		source       []byte
		expectedName string
	}{
		{
			name:   "text - no name",
			source: []byte(`(module)`),
		},
		{
			name:   "text - empty name",
			source: []byte(`(module $)`),
		},
		{
			name:         "text - name",
			source:       []byte(`(module $test)`),
			expectedName: "test",
		},
		{
			name:   "binary - no name section",
			source: binary.EncodeModule(&wasm.Module{}),
		},
		{
			name:   "binary - empty NameSection.ModuleName",
			source: binary.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{}}),
		},
		{
			name:         "binary - NameSection.ModuleName",
			source:       binary.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "test"}}),
			expectedName: "test",
		},
	}

	r := NewRuntime()
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			code, err := r.CompileModule(testCtx, tc.source)
			require.NoError(t, err)
			defer code.Close(testCtx)
			if tc.expectedName != "" {
				require.Equal(t, tc.expectedName, code.module.NameSection.ModuleName)
			}
			require.Equal(t, r.(*runtime).store.Engine, code.compiledEngine)
		})
	}

	t.Run("text - memory", func(t *testing.T) {
		r := NewRuntimeWithConfig(NewRuntimeConfig().
			WithMemoryCapacityPages(func(minPages uint32, maxPages *uint32) uint32 { return 2 }))

		source := []byte(`(module (memory 1 3))`)

		code, err := r.CompileModule(testCtx, source)
		require.NoError(t, err)
		defer code.Close(testCtx)

		require.Equal(t, &wasm.Memory{
			Min:          1,
			Cap:          2, // Uses capacity function
			Max:          3,
			IsMaxEncoded: true,
		}, code.module.MemorySection)
	})
}

func TestRuntime_CompileModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		runtime     Runtime
		source      []byte
		expectedErr string
	}{
		{
			name:        "nil",
			expectedErr: "source == nil",
		},
		{
			name:        "invalid binary",
			source:      append(binary.Magic, []byte("yolo")...),
			expectedErr: "invalid version header",
		},
		{
			name:        "invalid text",
			source:      []byte(`(modular)`),
			expectedErr: "1:2: unexpected field: modular",
		},
		{
			name:        "RuntimeConfig.memoryLimitPages too large",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryLimitPages(math.MaxUint32)),
			source:      []byte(`(module)`),
			expectedErr: "memoryLimitPages 4294967295 (3 Ti) > specification max 65536 (4 Gi)",
		},
		{
			name:        "memory has too many pages - text",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryLimitPages(2)),
			source:      []byte(`(module (memory 3))`),
			expectedErr: "1:17: min 3 pages (192 Ki) over limit of 2 pages (128 Ki) in module.memory[0]",
		},
		{
			name: "memory cap < min", // only one test to avoid duplicating tests in module_test.go
			runtime: NewRuntimeWithConfig(NewRuntimeConfig().
				WithMemoryCapacityPages(func(minPages uint32, maxPages *uint32) uint32 { return 1 })),
			source:      []byte(`(module (memory 3))`),
			expectedErr: "memory[0] capacity 1 pages (64 Ki) less than minimum 3 pages (192 Ki)",
		},
		{
			name: "memory cap < min - exported", // only one test to avoid duplicating tests in module_test.go
			runtime: NewRuntimeWithConfig(NewRuntimeConfig().
				WithMemoryCapacityPages(func(minPages uint32, maxPages *uint32) uint32 { return 1 })),
			source:      []byte(`(module (memory 3) (export "memory" (memory 0)))`),
			expectedErr: "memory[memory] capacity 1 pages (64 Ki) less than minimum 3 pages (192 Ki)",
		},
		{
			name:        "memory has too many pages - binary",
			runtime:     NewRuntimeWithConfig(NewRuntimeConfig().WithMemoryLimitPages(2)),
			source:      binary.EncodeModule(&wasm.Module{MemorySection: &wasm.Memory{Min: 2, Max: 3, IsMaxEncoded: true}}),
			expectedErr: "section memory: max 3 pages (192 Ki) over limit of 2 pages (128 Ki)",
		},
	}

	r := NewRuntime()
	for _, tt := range tests {
		tc := tt

		if tc.runtime == nil {
			tc.runtime = r
		}

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.runtime.CompileModule(testCtx, tc.source)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestRuntime_setMemoryCapacity(t *testing.T) {
	tests := []struct {
		name        string
		runtime     *runtime
		mem         *wasm.Memory
		expectedErr string
	}{
		{
			name: "cap ok",
			runtime: &runtime{memoryCapacityPages: func(minPages uint32, maxPages *uint32) uint32 {
				return 3
			}, memoryLimitPages: 3},
			mem: &wasm.Memory{Min: 2},
		},
		{
			name: "cap < min",
			runtime: &runtime{memoryCapacityPages: func(minPages uint32, maxPages *uint32) uint32 {
				return 1
			}, memoryLimitPages: 3},
			mem:         &wasm.Memory{Min: 2},
			expectedErr: "memory[memory] capacity 1 pages (64 Ki) less than minimum 2 pages (128 Ki)",
		},
		{
			name: "cap > maxLimit",
			runtime: &runtime{memoryCapacityPages: func(minPages uint32, maxPages *uint32) uint32 {
				return 4
			}, memoryLimitPages: 3},
			mem:         &wasm.Memory{Min: 2},
			expectedErr: "memory[memory] capacity 4 pages (256 Ki) over limit of 3 pages (192 Ki)",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			err := tc.runtime.setMemoryCapacity("memory", tc.mem)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
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

		r := NewRuntime()
		t.Run(tc.name, func(t *testing.T) {
			// Instantiate the module and get the export of the above memory
			module, err := tc.builder(r).Instantiate(testCtx)
			require.NoError(t, err)
			defer module.Close(testCtx)

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

		r := NewRuntime().(*runtime)
		t.Run(tc.name, func(t *testing.T) {
			var code *CompiledCode
			if tc.module != nil {
				code = &CompiledCode{module: tc.module}
			} else {
				code, _ = tc.builder(r).Build(testCtx)
			}

			err := r.store.Engine.CompileModule(testCtx, code.module)
			require.NoError(t, err)

			// Instantiate the module and get the export of the above global
			module, err := r.InstantiateModule(testCtx, code)
			require.NoError(t, err)
			defer module.Close(testCtx)

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

func TestFunction_Context(t *testing.T) {
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

			// Define a host function so that we can catch the context propagated from a module function call
			functionName := "fn"
			expectedResult := uint64(math.MaxUint64)
			hostFn := func(ctx context.Context) uint64 {
				require.Equal(t, tc.expected, ctx)
				return expectedResult
			}
			source, closer := requireImportAndExportFunction(t, r, hostFn, functionName)
			defer closer(testCtx) // nolint

			// Instantiate the module and get the export of the above hostFn
			module, err := r.InstantiateModuleFromCodeWithConfig(tc.ctx, source, NewModuleConfig().WithName(t.Name()))
			require.NoError(t, err)
			defer module.Close(testCtx)

			// This fails if the function wasn't invoked, or had an unexpected context.
			results, err := module.ExportedFunction(functionName).Call(tc.ctx)
			require.NoError(t, err)
			require.Equal(t, expectedResult, results[0])
		})
	}
}

func TestRuntime_InstantiateModule_UsesContext(t *testing.T) {
	r := NewRuntime()

	// Define a function that will be set as the start function
	var calledStart bool
	start := func(ctx context.Context) {
		calledStart = true
		require.Equal(t, testCtx, ctx)
	}

	env, err := r.NewModuleBuilder("env").
		ExportFunction("start", start).
		Instantiate(testCtx)
	require.NoError(t, err)
	defer env.Close(testCtx)

	code, err := r.CompileModule(testCtx, []byte(`(module $runtime_test.go
	(import "env" "start" (func $start))
	(start $start)
)`))
	require.NoError(t, err)
	defer code.Close(testCtx)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	m, err := r.InstantiateModule(testCtx, code)
	require.NoError(t, err)
	defer m.Close(testCtx)

	require.True(t, calledStart)
}

// TestInstantiateModuleFromCode_DoesntEnforce_Start ensures wapc-go work when modules import WASI, but don't export "_start".
func TestInstantiateModuleFromCode_DoesntEnforce_Start(t *testing.T) {
	r := NewRuntime()

	mod, err := r.InstantiateModuleFromCode(testCtx, []byte(`(module $wasi_test.go
	(memory 1)
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)
	require.NoError(t, mod.Close(testCtx))
}

func TestRuntime_InstantiateModuleFromCode_UsesContext(t *testing.T) {
	r := NewRuntime()

	// Define a function that will be re-exported as the WASI function: _start
	var calledStart bool
	start := func(ctx context.Context) {
		calledStart = true
		require.Equal(t, testCtx, ctx)
	}

	host, err := r.NewModuleBuilder("").
		ExportFunction("start", start).
		Instantiate(testCtx)
	require.NoError(t, err)
	defer host.Close(testCtx)

	// Start the module as a WASI command. This will fail if the context wasn't as intended.
	mod, err := r.InstantiateModuleFromCode(testCtx, []byte(`(module $start
	(import "" "start" (func $start))
	(memory 1)
	(export "_start" (func $start))
	(export "memory" (memory 0))
)`))
	require.NoError(t, err)
	defer mod.Close(testCtx)

	require.True(t, calledStart)
}

// TestInstantiateModuleWithConfig_WithName tests that we can pre-validate (cache) a module and instantiate it under
// different names. This pattern is used in wapc-go.
func TestInstantiateModuleWithConfig_WithName(t *testing.T) {
	r := NewRuntime()
	base, err := r.CompileModule(testCtx, []byte(`(module $0 (memory 1))`))
	require.NoError(t, err)
	defer base.Close(testCtx)

	require.Equal(t, "0", base.module.NameSection.ModuleName)

	// Use the same runtime to instantiate multiple modules
	internal := r.(*runtime).store
	m1, err := r.InstantiateModuleWithConfig(testCtx, base, NewModuleConfig().WithName("1"))
	require.NoError(t, err)
	defer m1.Close(testCtx)

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("1"), m1)

	m2, err := r.InstantiateModuleWithConfig(testCtx, base, NewModuleConfig().WithName("2"))
	require.NoError(t, err)
	defer m2.Close(testCtx)

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("2"), m2)
}

func TestInstantiateModuleWithConfig_ExitError(t *testing.T) {
	r := NewRuntime()

	start := func(ctx context.Context, m api.Module) {
		require.NoError(t, m.CloseWithExitCode(ctx, 2))
	}

	_, err := r.NewModuleBuilder("env").ExportFunction("_start", start).Instantiate(testCtx)

	// Ensure the exit error propagated and didn't wrap.
	require.Equal(t, err, sys.NewExitError("env", 2))
}

// requireImportAndExportFunction re-exports a host function because only host functions can see the propagated context.
func requireImportAndExportFunction(t *testing.T, r Runtime, hostFn func(ctx context.Context) uint64, functionName string) ([]byte, func(context.Context) error) {
	mod, err := r.NewModuleBuilder("host").ExportFunction(functionName, hostFn).Instantiate(testCtx)
	require.NoError(t, err)

	return []byte(fmt.Sprintf(
		`(module (import "host" "%[1]s" (func (result i64))) (export "%[1]s" (func 0)))`, functionName,
	)), mod.Close
}

type mockEngine struct {
	name          string
	cachedModules map[*wasm.Module]struct{}
}

// NewModuleEngine implements the same method as documented on wasm.Engine.
func (e *mockEngine) NewModuleEngine(_ string, _ *wasm.Module, _, _ []*wasm.FunctionInstance, _ *wasm.TableInstance, _ map[wasm.Index]wasm.Index) (wasm.ModuleEngine, error) {
	return nil, nil
}

// DeleteCompiledModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) DeleteCompiledModule(module *wasm.Module) {
	delete(e.cachedModules, module)
}

func (e *mockEngine) CompileModule(_ context.Context, module *wasm.Module) error {
	e.cachedModules[module] = struct{}{}
	return nil
}
