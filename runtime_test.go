package wazero

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

type arbitrary struct{}

var (
	binaryNamedZero = binaryencoding.EncodeModule(&wasm.Module{NameSection: &wasm.NameSection{ModuleName: "0"}})
	// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
	testCtx = context.WithValue(context.Background(), arbitrary{}, "arbitrary")
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

func TestRuntime_CompileModule(t *testing.T) {
	tests := []struct {
		name          string
		runtime       Runtime
		wasm          *wasm.Module
		moduleBuilder HostModuleBuilder
		expected      func(CompiledModule)
	}{
		{
			name: "no name section",
			wasm: &wasm.Module{},
		},
		{
			name: "empty NameSection.ModuleName",
			wasm: &wasm.Module{NameSection: &wasm.NameSection{}},
		},
		{
			name: "NameSection.ModuleName",
			wasm: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "test"}},
			expected: func(compiled CompiledModule) {
				require.Equal(t, "test", compiled.Name())
			},
		},
		{
			name: "FunctionSection, but not exported",
			wasm: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			},
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedFunctions())
				require.Zero(t, len(compiled.ExportedFunctions()))
			},
		},
		{
			name: "FunctionSection exported",
			wasm: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []api.ValueType{api.ValueTypeI32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
				ExportSection: []wasm.Export{{
					Type:  wasm.ExternTypeFunc,
					Name:  "function",
					Index: 0,
				}},
			},
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedFunctions())
				f := compiled.ExportedFunctions()["function"]
				require.Equal(t, []api.ValueType{api.ValueTypeI32}, f.ParamTypes())
			},
		},
		{
			name: "MemorySection, but not exported",
			wasm: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 2, Max: 3, IsMaxEncoded: true},
			},
			expected: func(compiled CompiledModule) {
				require.Nil(t, compiled.ImportedMemories())
				require.Zero(t, len(compiled.ExportedMemories()))
			},
		},
		{
			name: "MemorySection exported",
			wasm: &wasm.Module{
				MemorySection: &wasm.Memory{Min: 2, Max: 3, IsMaxEncoded: true},
				ExportSection: []wasm.Export{{
					Type:  wasm.ExternTypeMemory,
					Name:  "memory",
					Index: 0,
				}},
			},
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

	_r := NewRuntime(testCtx)
	defer _r.Close(testCtx)

	r := _r.(*runtime)

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bin := binaryencoding.EncodeModule(tc.wasm)

			m, err := r.CompileModule(testCtx, bin)
			require.NoError(t, err)
			if tc.expected == nil {
				tc.expected = func(CompiledModule) {}
			}
			tc.expected(m)
			require.Equal(t, r.store.Engine, m.(*compiledModule).compiledEngine)

			// TypeIDs must be assigned to compiledModule.
			expTypeIDs, err := r.store.GetFunctionTypeIDs(tc.wasm.TypeSection)
			require.NoError(t, err)
			require.Equal(t, expTypeIDs, m.(*compiledModule).typeIDs)
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
			expectedErr: "invalid magic number",
		},
		{
			name:        "invalid binary",
			wasm:        append(binaryencoding.Magic, []byte("yolo")...),
			expectedErr: "invalid version header",
		},
		{
			name:        "memory has too many pages",
			wasm:        binaryencoding.EncodeModule(&wasm.Module{MemorySection: &wasm.Memory{Min: 2, Cap: 2, Max: 70000, IsMaxEncoded: true}}),
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
			wasm: binaryencoding.EncodeModule(&wasm.Module{}),
		},
		{
			name: "memory exported, one page",
			wasm: binaryencoding.EncodeModule(&wasm.Module{
				MemorySection: &wasm.Memory{Min: 1},
				ExportSection: []wasm.Export{{Name: "memory", Type: api.ExternTypeMemory}},
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
			module, err := r.Instantiate(testCtx, tc.wasm)
			require.NoError(t, err)

			mem := module.ExportedMemory("memory")
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
				defs := module.ExportedMemoryDefinitions()
				require.Equal(t, 1, len(defs))
				def := defs["memory"]
				require.Equal(t, tc.expectedLen>>16, def.Min())
			} else {
				require.Nil(t, mem)
				require.Zero(t, len(module.ExportedMemoryDefinitions()))
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
				GlobalSection: []wasm.Global{
					{
						Type: wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
						Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
			},
		},
		{
			name: "global exported",
			module: &wasm.Module{
				GlobalSection: []wasm.Global{
					{
						Type: wasm.GlobalType{ValType: wasm.ValueTypeI64},
						Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
				Exports: map[string]*wasm.Export{
					"global": {Type: wasm.ExternTypeGlobal, Name: "global"},
				},
			},
			expected: true,
		},
		{
			name: "global exported and mutable",
			module: &wasm.Module{
				GlobalSection: []wasm.Global{
					{
						Type: wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
						Init: wasm.ConstantExpression{Opcode: wasm.OpcodeI64Const, Data: leb128.EncodeInt64(globalVal)},
					},
				},
				Exports: map[string]*wasm.Export{
					"global": {Type: wasm.ExternTypeGlobal, Name: "global"},
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

			err := r.store.Engine.CompileModule(testCtx, code.module, nil, false)
			require.NoError(t, err)

			// Instantiate the module and get the export of the above global
			module, err := r.InstantiateModule(testCtx, code, NewModuleConfig())
			require.NoError(t, err)

			global := module.ExportedGlobal("global")
			if !tc.expected {
				require.Nil(t, global)
				return
			}
			require.Equal(t, uint64(globalVal), global.Get())

			mutable, ok := global.(api.MutableGlobal)
			require.Equal(t, tc.expectedMutable, ok)
			if ok {
				mutable.Set(2)
				require.Equal(t, uint64(2), global.Get())
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
		Instantiate(testCtx)
	require.NoError(t, err)

	one := uint32(1)
	binary := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		ImportSection:   []wasm.Import{{Module: "env", Name: "start", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{
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

// TestRuntime_Instantiate_DoesntEnforce_Start ensures wapc-go work when modules import WASI, but don't
// export "_start".
func TestRuntime_Instantiate_DoesntEnforce_Start(t *testing.T) {
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	binary := binaryencoding.EncodeModule(&wasm.Module{
		MemorySection: &wasm.Memory{Min: 1},
		ExportSection: []wasm.Export{{Name: "memory", Type: wasm.ExternTypeMemory, Index: 0}},
	})

	mod, err := r.Instantiate(testCtx, binary)
	require.NoError(t, err)
	require.NoError(t, mod.Close(testCtx))
}

func TestRuntime_Instantiate_ErrorOnStart(t *testing.T) {
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

			start := func() {
				panic(errors.New("ice cream"))
			}

			host, err := r.NewHostModuleBuilder("host").
				NewFunctionBuilder().WithFunc(start).Export("start").
				Instantiate(testCtx)
			require.NoError(t, err)

			// Start the module as a WASI command. We expect it to fail.
			_, err = r.Instantiate(testCtx, []byte(tc.wasm))
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
	internal := r.(*runtime)
	m1, err := r.InstantiateModule(testCtx, base, NewModuleConfig().WithName("1"))
	require.NoError(t, err)
	require.Equal(t, "1", m1.Name())

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("1"), m1)

	m2, err := r.InstantiateModule(testCtx, base, NewModuleConfig().WithName("2"))
	require.NoError(t, err)
	require.Equal(t, "2", m2.Name())

	require.Nil(t, internal.Module("0"))
	require.Equal(t, internal.Module("2"), m2)

	// Empty name module shouldn't be returned via Module() for future optimization.
	m3, err := r.InstantiateModule(testCtx, base, NewModuleConfig().WithName(""))
	require.NoError(t, err)
	require.Equal(t, "", m3.Name())

	ret := internal.Module("")
	require.Nil(t, ret)
}

func TestRuntime_InstantiateModule_ExitError(t *testing.T) {
	r := NewRuntime(testCtx)
	defer r.Close(testCtx)

	tests := []struct {
		name        string
		exitCode    uint32
		export      bool
		expectedErr error
	}{
		{
			name:        "start: exit code 0",
			exitCode:    0,
			expectedErr: sys.NewExitError(0),
		},
		{
			name:        "start: exit code 2",
			exitCode:    2,
			expectedErr: sys.NewExitError(2),
		},
		{
			name:     "_start: exit code 0",
			exitCode: 0,
			export:   true,
		},
		{
			name:        "_start: exit code 2",
			exitCode:    2,
			export:      true,
			expectedErr: sys.NewExitError(2),
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			start := func(ctx context.Context, m api.Module) {
				require.NoError(t, m.CloseWithExitCode(ctx, tc.exitCode))
			}

			env, err := r.NewHostModuleBuilder("env").
				NewFunctionBuilder().WithFunc(start).Export("exit").
				Instantiate(testCtx)
			require.NoError(t, err)
			defer env.Close(testCtx)

			mod := &wasm.Module{
				TypeSection:     []wasm.FunctionType{{}},
				ImportSection:   []wasm.Import{{Module: "env", Name: "exit", Type: wasm.ExternTypeFunc, DescFunc: 0}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Call the imported env.start.
				},
			}
			if tc.export {
				mod.ExportSection = []wasm.Export{
					{Name: "_start", Type: wasm.ExternTypeFunc, Index: 1},
				}
			} else {
				one := uint32(1)
				mod.StartSection = &one
			}
			binary := binaryencoding.EncodeModule(mod)

			// Instantiate the module, which calls the start function.
			m, err := r.InstantiateWithConfig(testCtx, binary,
				NewModuleConfig().WithName("call-exit"))

			// Ensure the exit error propagated and didn't wrap.
			require.Equal(t, tc.expectedErr, err)

			// Ensure calling close again doesn't break
			if err == nil {
				require.NoError(t, m.Close(testCtx))
			}
		})
	}
}

func TestRuntime_CloseWithExitCode(t *testing.T) {
	bin := binaryencoding.EncodeModule(&wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Index: 0, Name: "func"}},
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
			require.Equal(t, map[string]api.FunctionDefinition{"func": func1.Definition()},
				m1.ExportedFunctionDefinitions())
			func2 := m2.ExportedFunction("func")
			require.Equal(t, map[string]api.FunctionDefinition{"func": func2.Definition()},
				m2.ExportedFunctionDefinitions())

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
			require.ErrorIs(t, err, sys.NewExitError(tc.exitCode))

			_, err = func2.Call(testCtx)
			require.ErrorIs(t, err, sys.NewExitError(tc.exitCode))
		})
	}
}

func TestHostFunctionWithCustomContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config RuntimeConfig
	}{
		{name: "compiler", config: NewRuntimeConfigCompiler()},
		{name: "interpreter", config: NewRuntimeConfigInterpreter()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const fistString = "hello"
			const secondString = "hello call"
			hostCtx := &HostContext{fistString}
			r := NewRuntimeWithConfig(hostCtx, tc.config)
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
				Instantiate(hostCtx)
			require.NoError(t, err)

			startFnIndex := uint32(2)
			binary := binaryencoding.EncodeModule(&wasm.Module{
				TypeSection: []wasm.FunctionType{{}},
				ImportSection: []wasm.Import{
					{Module: "env", Name: "host", Type: wasm.ExternTypeFunc, DescFunc: 0},
					{Module: "env", Name: "host2", Type: wasm.ExternTypeFunc, DescFunc: 0},
				},
				FunctionSection: []wasm.Index{0, 0},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Call the imported env.host.
					{Body: []byte{wasm.OpcodeCall, 1, wasm.OpcodeEnd}}, // Call the imported env.host.
				},
				ExportSection: []wasm.Export{
					{Type: api.ExternTypeFunc, Name: "callHost", Index: uint32(3)},
				},
				StartSection: &startFnIndex,
			})

			// Instantiate the module, which calls the start function. This will fail if the context wasn't as intended.
			ins, err := r.Instantiate(hostCtx, binary)
			require.NoError(t, err)
			require.True(t, calledStart)

			// add the new context content for call with used in host function
			hostCtx.Content = secondString
			_, err = ins.ExportedFunction("callHost").Call(hostCtx)
			require.NoError(t, err)
			require.True(t, calledCall)
		})
	}
}

func TestRuntime_Close_ClosesCompiledModules(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		withCompilationCache bool
	}{
		{name: "with cache", withCompilationCache: true},
		{name: "without cache", withCompilationCache: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &mockEngine{name: "mock", cachedModules: map[*wasm.Module]struct{}{}}
			conf := *engineLessConfig
			conf.newEngine = func(context.Context, api.CoreFeatures, filecache.Cache) wasm.Engine { return engine }
			if tc.withCompilationCache {
				conf.cache = NewCompilationCache()
			}
			r := NewRuntimeWithConfig(testCtx, &conf)
			defer r.Close(testCtx)

			// Normally compiled modules are closed when instantiated but this is never instantiated.
			_, err := r.CompileModule(testCtx, binaryNamedZero)
			require.NoError(t, err)
			require.Equal(t, uint32(1), engine.CompiledModuleCount())

			err = r.Close(testCtx)
			require.NoError(t, err)

			// Closing the runtime should remove the compiler cache if cache is not configured.
			require.Equal(t, !tc.withCompilationCache, engine.closed)
		})
	}
}

// TestRuntime_Closed ensures invocation of closed Runtime's methods is safe.
func TestRuntime_Closed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		errFunc func(r Runtime, mod CompiledModule) error
	}{
		{
			name: "InstantiateModule",
			errFunc: func(r Runtime, mod CompiledModule) error {
				_, err := r.InstantiateModule(testCtx, mod, NewModuleConfig())
				return err
			},
		},
		{
			name: "Instantiate",
			errFunc: func(r Runtime, mod CompiledModule) error {
				_, err := r.Instantiate(testCtx, binaryNamedZero)
				return err
			},
		},
		{
			name: "CompileModule",
			errFunc: func(r Runtime, mod CompiledModule) error {
				_, err := r.CompileModule(testCtx, binaryNamedZero)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &mockEngine{name: "mock", cachedModules: map[*wasm.Module]struct{}{}}
			conf := *engineLessConfig
			conf.newEngine = func(context.Context, api.CoreFeatures, filecache.Cache) wasm.Engine { return engine }
			r := NewRuntimeWithConfig(testCtx, &conf)
			defer r.Close(testCtx)

			// Normally compiled modules are closed when instantiated but this is never instantiated.
			mod, err := r.CompileModule(testCtx, binaryNamedZero)
			require.NoError(t, err)
			require.Equal(t, uint32(1), engine.CompiledModuleCount())

			err = r.Close(testCtx)
			require.NoError(t, err)

			// Closing the runtime should remove the compiler cache if cache is not configured.
			require.True(t, engine.closed)

			require.EqualError(t, tc.errFunc(r, mod), "runtime closed with exit_code(0)")
		})
	}
}

type mockEngine struct {
	name          string
	cachedModules map[*wasm.Module]struct{}
	closed        bool
}

// CompileModule implements the same method as documented on wasm.Engine.
func (e *mockEngine) CompileModule(_ context.Context, module *wasm.Module, _ []experimental.FunctionListener, _ bool) error {
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
func (e *mockEngine) NewModuleEngine(_ *wasm.Module, _ *wasm.ModuleInstance) (wasm.ModuleEngine, error) {
	return nil, nil
}

// NewModuleEngine implements the same method as documented on wasm.Close.
func (e *mockEngine) Close() (err error) {
	e.closed = true
	return
}

// TestNewRuntime_concurrent ensures that concurrent execution of NewRuntime is race-free.
// This depends on -race flag.
func TestNewRuntime_concurrent(t *testing.T) {
	const num = 100
	var wg sync.WaitGroup
	c := NewCompilationCache()
	// If available, uses two engine configurations for the single compilation cache.
	configs := [2]RuntimeConfig{NewRuntimeConfigInterpreter().WithCompilationCache(c)}
	if platform.CompilerSupported() {
		configs[1] = NewRuntimeConfigCompiler().WithCompilationCache(c)
	} else {
		configs[1] = NewRuntimeConfigInterpreter().WithCompilationCache(c)
	}
	wg.Add(num)
	for i := 0; i < num; i++ {
		i := i
		go func() {
			defer wg.Done()
			r := NewRuntimeWithConfig(testCtx, configs[i%2])
			err := r.Close(testCtx)
			require.NoError(t, err)
		}()
	}
	wg.Wait()
}
