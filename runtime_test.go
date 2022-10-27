package wazero

import (
	"context"
	_ "embed"
	"errors"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/internal/wasm"
	binaryformat "github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/sys"
)

var (
	binaryNamedZero = binaryformat.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "0"}})
	// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
	testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")
)

var _ context.Context = &HostContext{}

// HostContext contain the content will be used in host function call
type HostContext struct {
	Content string
}

func (h *HostContext) Deadline() (deadline time.Time, ok bool) { return }

func (h *HostContext) Done() <-chan struct{} { return nil }

func (h *HostContext) Err() error { return nil }

func (h *HostContext) Value(key interface{}) interface{} { return nil }

func TestNewRuntimeWithConfig_version(t *testing.T) {
	cfg := NewRuntimeConfig().(*runtimeConfig)
	oldNewEngine := cfg.newEngine
	cfg.newEngine = func(ctx context.Context, features api.CoreFeatures) wasm.Engine {
		// Ensures that wazeroVersion is propagated to the engine.
		v := ctx.Value(version.WazeroVersionKey{})
		require.NotNil(t, v)
		require.Equal(t, wazeroVersion, v.(string))
		return oldNewEngine(ctx, features)
	}
	_ = NewRuntimeWithConfig(testCtx, cfg)
}

func TestRuntime_CompileModule(t *testing.T) {
	tests := []struct {
		name          string
		runtime       Runtime
		wasm          []byte
		moduleBuilder HostModuleBuilder
		expected      func(CompiledModule)
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
			name: "NameSection.ModuleName",
			wasm: binaryformat.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "test"}}),
			expected: func(compiled CompiledModule) {
				require.Equal(t, "test", compiled.Name())
			},
		},
		{
			name: "FunctionSection, but not exported",
			wasm: binaryformat.EncodeModule(&wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			}),
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedFunctions())
				require.Zero(t, len(compiled.ExportedFunctions()))
			},
		},
		{
			name: "FunctionSection exported",
			wasm: binaryformat.EncodeModule(&wasm.Module{
				TypeSection:     []*wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
				ExportSection: []*wasm.Export{{
					Type:  wasm.ExternTypeFunc,
					Name:  "function",
					Index: 0,
				}},
			}),
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedFunctions())
				f := compiled.ExportedFunctions()["function"]
				require.Equal(t, []api.ValueType{api.ValueTypeI32}, f.ParamTypes())
			},
		},
		{
			name: "MemorySection, but not exported",
			wasm: binaryformat.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{Min: 2, Max: 3, IsMaxEncoded: true},
			}),
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedMemories())
				require.Zero(t, len(compiled.ExportedMemories()))
			},
		},
		{
			name: "MemorySection exported",
			wasm: binaryformat.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				ExportSection: []*wasm.Export{{
					Type:  wasm.ExternTypeMemory,
					Name:  "memory",
					Index: 0,
				}},
			}),
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedMemories())
				mem := compiled.ExportedMemories()["memory"]
				require.Equal(t, uint32(2), mem.Min())
				max, ok := mem.Max()
				require.Equal(t, uint32(3), max)
				require.True(t, ok)
			},
		},
	}

	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, err := r.CompileModule(testCtx, tc.wasm)
			require.NoError(t, err)
			if tc.expected == nil {
				tc.expected = func(CompiledModule) {}
			}
			tc.expected(m)
			require.Equal(t, r.(*runtime).store.Engine, m.(*compiledModule).compiledEngine)
		})
	}
}

func TestRuntime_CompileModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
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
			name:        "memory has too many pages",
			wasm:        binaryformat.EncodeModule(&wasm.Module{MemorySection: &wasm.Memory{Min: 2, Cap: 2, Max: 70000, IsMaxEncoded: true}}),
			expectedErr: "section memory: max 70000 pages (4 Gi) over limit of 65536 pages (4 Gi)",
		},
	}

	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := r.CompileModule(testCtx, tc.wasm)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// TestModule_Memory only covers a couple cases to avoid duplication of internal/wasm/runtime_test.go
func TestModule_Memory(t *testing.T) {
	tests := []struct {
		name        string
		wasm        []byte
		expected    bool
		expectedLen uint32
	}{
		{
			name: "no memory",
			wasm: binaryformat.EncodeModule(&wasm.Module{}),
		},
		{
			name: "memory exported, one page",
			wasm: binaryformat.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{Min: 1},
				ExportSection: []*wasm.Export{{Name: "memory", Type: api.ExternTypeMemory}},
			}),
			expected:    true,
			expectedLen: 65536,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			r := NewRuntime(testCtx)
			defer r.Close(testCtx)

			// Instantiate the module and get the export of the above memory
			module, err := r.InstantiateModuleFromBinary(testCtx, tc.wasm)
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
			module: &wasm.Module{
				GlobalSection: []*wasm.Global{
					{
						Type: &wasm.GlobalType{ValType: wasm.ValueTypeI64},
						Init: &wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
				ExportSection: []*wasm.Export{
					{Type: wasm.ExternTypeGlobal, Name: "global"},
				},
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
			r := NewRuntime(testCtx).(*runtime)
			defer r.Close(testCtx)

			code := &compiledModule{module: tc.module}

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

func TestRuntime_InstantiateModule_UsesContext(t *testing.T) {
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	// Define a function that will be set as the start function
	var calledStart bool
	start := func(ctx context.Context) {
		calledStart = true
		require.Equal(t, testCtx, ctx)
	}

	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(start).Export("start").
		Instantiate(testCtx, r)
	require.NoError(t, err)

	one := uint32(1)
	binary := binaryformat.EncodeModule(&wasm.Module{
		TypeSection:     []*wasm.FunctionType{{}},
		ImportSection:   []*wasm.Import{{Module: "env", Name: "start", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Call the imported env.start.
		},
		StartSection: &one,
	})

	code, err := r.CompileModule(testCtx, binary)
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
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	binary := binaryformat.EncodeModule(&wasm.Module{
		MemorySection: &wasm.Memory{Min: 1},
		ExportSection: []*wasm.Export{{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0}},
	})

	mod, err := r.InstantiateModuleFromBinary(testCtx, binary)
	require.NoError(t, err)
	require.NoError(t, mod.Close(testCtx))
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
			r := NewRuntime(testCtx)
			defer r.Close(testCtx)

			start := func(context.Context) {
				panic(errors.New("ice cream"))
			}

			host, err := r.NewHostModuleBuilder("").
				NewFunctionBuilder().WithFunc(start).Export("start").
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
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	base, err := r.CompileModule(testCtx, binaryNamedZero)
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
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	start := func(ctx context.Context, m api.Module) {
		require.NoError(t, m.CloseWithExitCode(ctx, 2))
	}

	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(start).Export("exit").
		Instantiate(testCtx, r)
	require.NoError(t, err)

	one := uint32(1)
	binary := binaryformat.EncodeModule(&wasm.Module{
		TypeSection:     []*wasm.FunctionType{{}},
		ImportSection:   []*wasm.Import{{Module: "env", Name: "exit", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Call the imported env.start.
		},
		StartSection: &one,
	})

	code, err := r.CompileModule(testCtx, binary)
	require.NoError(t, err)

	// Instantiate the module, which calls the start function.
	_, err = r.InstantiateModule(testCtx, code, NewModuleConfig().WithName("call-exit"))

	// Ensure the exit error propagated and didn't wrap.
	require.Equal(t, err, sys.NewExitError("call-exit", 2))
}

func TestRuntime_CloseWithExitCode(t *testing.T) {
	bin := binaryformat.EncodeModule(&wasm.Module{
		TypeSection:     []*wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []*wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
		ExportSection:   []*wasm.Export{{Type: wasm.ExternTypeFunc, Index: 0, Name: "func"}},
	})

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
			r := NewRuntime(testCtx)

			code, err := r.CompileModule(testCtx, bin)
			require.NoError(t, err)

			// Instantiate two modules.
			m1, err := r.InstantiateModule(testCtx, code, NewModuleConfig().WithName("mod1"))
			require.NoError(t, err)
			m2, err := r.InstantiateModule(testCtx, code, NewModuleConfig().WithName("mod2"))
			require.NoError(t, err)

			func1 := m1.ExportedFunction("func")
			func2 := m2.ExportedFunction("func")

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

func TestHostFunctionWithCustomContext(t *testing.T) {
	const fistString = "hello"
	const secondString = "hello call"
	hostCtx := &HostContext{fistString}
	r := NewRuntime(hostCtx)
	defer r.Close(hostCtx)

	// Define a function that will be set as the start function
	var calledStart bool
	var calledCall bool
	start := func(ctx context.Context, module api.Module) {
		hts, ok := ctx.(*HostContext)
		if !ok {
			t.Fatal("decorate call context could effect host ctx cast failed, please consider it.")
		}
		calledStart = true
		require.NotNil(t, hts)
		require.Equal(t, fistString, hts.Content)
	}

	callFunc := func(ctx context.Context, module api.Module) {
		hts, ok := ctx.(*HostContext)
		if !ok {
			t.Fatal("decorate call context could effect host ctx cast failed, please consider it.")
		}
		calledCall = true
		require.NotNil(t, hts)
		require.Equal(t, secondString, hts.Content)
	}

	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithFunc(start).Export("host").
		NewFunctionBuilder().WithFunc(callFunc).Export("host2").
		Instantiate(hostCtx, r)
	require.NoError(t, err)

	one := uint32(0)
	binary := binaryformat.EncodeModule(&wasm.Module{
		TypeSection: []*wasm.FunctionType{{}, {}},
		ImportSection: []*wasm.Import{
			{Module: "env", Name: "host", Type: wasm.ExternTypeFunc, DescFunc: 0},
			{Module: "env", Name: "host2", Type: wasm.ExternTypeFunc, DescFunc: 0},
		},
		FunctionSection: []wasm.Index{0, 1},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Call the imported env.host.
			{Body: []byte{wasm.OpcodeCall, 1, wasm.OpcodeEnd}}, // Call the imported env.host.
		},
		ExportSection: []*wasm.Export{
			{Type: api.ExternTypeFunc, Name: "callHost", Index: uint32(3)},
		},
		StartSection: &one,
	})

	code, err := r.CompileModule(hostCtx, binary)
	require.NoError(t, err)

	// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
	ins, err := r.InstantiateModule(hostCtx, code, NewModuleConfig())
	require.NoError(t, err)
	require.True(t, calledStart)

	// add the new context content for call with used in host function
	hostCtx.Content = secondString
	_, err = ins.ExportedFunction("callHost").Call(hostCtx)
	require.NoError(t, err)
	require.True(t, calledCall)
}

func TestRuntime_Close_ClosesCompiledModules(t *testing.T) {
	engine := &mockEngine{name: "mock", cachedModules: map[*wasm.Module]struct{}{}}
	conf := *engineLessConfig
	conf.newEngine = func(context.Context, api.CoreFeatures) wasm.Engine {
		return engine
	}
	r := NewRuntimeWithConfig(testCtx, &conf)
	defer r.Close(testCtx)

	// Normally compiled modules are closed when instantiated but this is never instantiated.
	_, err := r.CompileModule(testCtx, binaryNamedZero)
	require.NoError(t, err)
	require.Equal(t, uint32(1), engine.CompiledModuleCount())

	err = r.Close(testCtx)
	require.NoError(t, err)

	// Closing the runtime should remove the compiler cache
	require.Zero(t, engine.CompiledModuleCount())
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
