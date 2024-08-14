package logging_test

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/experimental/wazerotest"
	"github.com/tetratelabs/wazero/internal/testing/require"
	wasi "github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type arbitrary struct{}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), arbitrary{}, "arbitrary")

func Test_loggingListener(t *testing.T) {
	wasiFuncName := wasi.RandomGetName
	wasiFuncType := wasm.FunctionType{
		Params:  []api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
		Results: []api.ValueType{api.ValueTypeI32},
	}
	wasiParamNames := []string{"buf", "buf_len"}
	wasiParams := []uint64{0, 8}
	wasiResultNames := []string{"errno"}

	tests := []struct {
		name                    string
		moduleName, funcName    string
		functype                wasm.FunctionType
		isHostFunc              bool
		paramNames, resultNames []string
		params, results         []uint64
		expected                string
	}{
		{
			name:     "v_v",
			functype: wasm.FunctionType{},
			expected: `--> test.fn()
<--
`,
		},
		{
			name:       "host",
			functype:   wasm.FunctionType{},
			isHostFunc: true,
			expected: `==> test.fn()
<==
`,
		},
		{
			name:        "wasi",
			functype:    wasiFuncType,
			moduleName:  wasi.InternalModuleName,
			funcName:    wasiFuncName,
			paramNames:  wasiParamNames,
			resultNames: wasiResultNames,
			isHostFunc:  true,
			params:      wasiParams,
			results:     []uint64{uint64(wasi.ErrnoSuccess)},
			expected: `==> wasi_snapshot_preview1.random_get(buf=0,buf_len=8)
<== errno=ESUCCESS
`,
		},
		{
			name:        "wasi errno",
			functype:    wasiFuncType,
			moduleName:  wasi.InternalModuleName,
			funcName:    wasiFuncName,
			paramNames:  wasiParamNames,
			resultNames: wasiResultNames,
			isHostFunc:  true,
			params:      wasiParams,
			results:     []uint64{uint64(wasi.ErrnoFault)},
			expected: `==> wasi_snapshot_preview1.random_get(buf=0,buf_len=8)
<== errno=EFAULT
`,
		},
		{
			name:     "i32",
			functype: wasm.FunctionType{Params: []api.ValueType{api.ValueTypeI32}},
			params:   []uint64{math.MaxUint32},
			expected: `--> test.fn(-1)
<--
`,
		},
		{
			name:       "i32 named",
			functype:   wasm.FunctionType{Params: []api.ValueType{api.ValueTypeI32}},
			params:     []uint64{math.MaxUint32},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=-1)
<--
`,
		},
		{
			name:     "i64",
			functype: wasm.FunctionType{Params: []api.ValueType{api.ValueTypeI64}},
			params:   []uint64{math.MaxUint64},
			expected: `--> test.fn(-1)
<--
`,
		},
		{
			name:       "i64 named",
			functype:   wasm.FunctionType{Params: []api.ValueType{api.ValueTypeI64}},
			params:     []uint64{math.MaxUint64},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=-1)
<--
`,
		},
		{
			name:     "f32",
			functype: wasm.FunctionType{Params: []api.ValueType{api.ValueTypeF32}},
			params:   []uint64{api.EncodeF32(math.MaxFloat32)},
			expected: `--> test.fn(3.4028235e+38)
<--
`,
		},
		{
			name:       "f32 named",
			functype:   wasm.FunctionType{Params: []api.ValueType{api.ValueTypeF32}},
			params:     []uint64{api.EncodeF32(math.MaxFloat32)},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=3.4028235e+38)
<--
`,
		},
		{
			name:     "f64",
			functype: wasm.FunctionType{Params: []api.ValueType{api.ValueTypeF64}},
			params:   []uint64{api.EncodeF64(math.MaxFloat64)},
			expected: `--> test.fn(1.7976931348623157e+308)
<--
`,
		},
		{
			name:       "f64 named",
			functype:   wasm.FunctionType{Params: []api.ValueType{api.ValueTypeF64}},
			params:     []uint64{api.EncodeF64(math.MaxFloat64)},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=1.7976931348623157e+308)
<--
`,
		},
		{
			name:     "externref",
			functype: wasm.FunctionType{Params: []api.ValueType{api.ValueTypeExternref}},
			params:   []uint64{0},
			expected: `--> test.fn(0000000000000000)
<--
`,
		},
		{
			name:       "externref named",
			functype:   wasm.FunctionType{Params: []api.ValueType{api.ValueTypeExternref}},
			params:     []uint64{0},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=0000000000000000)
<--
`,
		},
		{
			name:     "v128",
			functype: wasm.FunctionType{Params: []api.ValueType{0x7b}},
			params:   []uint64{0, 1},
			expected: `--> test.fn(00000000000000000000000000000001)
<--
`,
		},
		{
			name:       "v128 named",
			functype:   wasm.FunctionType{Params: []api.ValueType{0x7b}},
			params:     []uint64{0, 1},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=00000000000000000000000000000001)
<--
`,
		},
		{
			name:     "funcref",
			functype: wasm.FunctionType{Params: []api.ValueType{0x70}},
			params:   []uint64{0},
			expected: `--> test.fn(0000000000000000)
<--
`,
		},
		{
			name:       "funcref named",
			functype:   wasm.FunctionType{Params: []api.ValueType{0x70}},
			params:     []uint64{0},
			paramNames: []string{"x"},
			expected: `--> test.fn(x=0000000000000000)
<--
`,
		},
		{
			name:     "no params, one result",
			functype: wasm.FunctionType{Results: []api.ValueType{api.ValueTypeI32}},
			results:  []uint64{math.MaxUint32},
			expected: `--> test.fn()
<-- -1
`,
		},
		{
			name: "one param, one result",
			functype: wasm.FunctionType{
				Params:  []api.ValueType{api.ValueTypeI32},
				Results: []api.ValueType{api.ValueTypeF32},
			},
			params:  []uint64{math.MaxUint32},
			results: []uint64{api.EncodeF32(math.MaxFloat32)},
			expected: `--> test.fn(-1)
<-- 3.4028235e+38
`,
		},
		{
			name: "two params, two results",
			functype: wasm.FunctionType{
				Params:  []api.ValueType{api.ValueTypeI32, api.ValueTypeI64},
				Results: []api.ValueType{api.ValueTypeF32, api.ValueTypeF64},
			},
			params:  []uint64{math.MaxUint32, math.MaxUint64},
			results: []uint64{api.EncodeF32(math.MaxFloat32), api.EncodeF64(math.MaxFloat64)},
			expected: `--> test.fn(-1,-1)
<-- (3.4028235e+38,1.7976931348623157e+308)
`,
		},
		{
			name: "two params, two results named",
			functype: wasm.FunctionType{
				Params:  []api.ValueType{api.ValueTypeI32, api.ValueTypeI64},
				Results: []api.ValueType{api.ValueTypeF32, api.ValueTypeF64},
			},
			params:     []uint64{math.MaxUint32, math.MaxUint64},
			paramNames: []string{"x", "y"},
			results:    []uint64{api.EncodeF32(math.MaxFloat32), api.EncodeF64(math.MaxFloat64)},
			expected: `--> test.fn(x=-1,y=-1)
<-- (3.4028235e+38,1.7976931348623157e+308)
`,
		},
	}

	var out bytes.Buffer
	lf := logging.NewLoggingListenerFactory(&out)
	fn := func() {}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if tc.moduleName == "" {
				tc.moduleName = "test"
			}
			if tc.funcName == "" {
				tc.funcName = "fn"
			}
			m := &wasm.Module{
				TypeSection:     []wasm.FunctionType{tc.functype},
				FunctionSection: []wasm.Index{0},
				NameSection: &wasm.NameSection{
					ModuleName:    tc.moduleName,
					FunctionNames: wasm.NameMap{{Name: tc.funcName}},
					LocalNames:    wasm.IndirectNameMap{{NameMap: toNameMap(tc.paramNames)}},
					ResultNames:   wasm.IndirectNameMap{{NameMap: toNameMap(tc.resultNames)}},
				},
			}

			if tc.isHostFunc {
				m.CodeSection = []wasm.Code{wasm.MustParseGoReflectFuncCode(fn)}
			} else {
				m.CodeSection = []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}}
			}
			def := m.FunctionDefinition(0)
			l := lf.NewFunctionListener(def)

			out.Reset()
			l.Before(testCtx, nil, def, tc.params, nil)
			l.After(testCtx, nil, def, tc.results)
			require.Equal(t, tc.expected, out.String())
		})
	}
}

func toNameMap(names []string) wasm.NameMap {
	if len(names) == 0 {
		return nil
	}
	var ret wasm.NameMap
	for i, n := range names {
		ret = append(ret, wasm.NameAssoc{Index: wasm.Index(i), Name: n})
	}
	return ret
}

func Test_loggingListener_indentation(t *testing.T) {
	out := bytes.NewBuffer(nil)
	lf := logging.NewLoggingListenerFactory(out)
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}, {Body: []byte{wasm.OpcodeEnd}}},
		NameSection: &wasm.NameSection{
			ModuleName:    "test",
			FunctionNames: wasm.NameMap{{Index: 0, Name: "fn1"}, {Index: 1, Name: "fn2"}},
		},
	}
	def1 := m.FunctionDefinition(0)
	l1 := lf.NewFunctionListener(def1)
	def2 := m.FunctionDefinition(1)
	l2 := lf.NewFunctionListener(def2)

	l1.Before(testCtx, nil, def1, []uint64{}, nil)
	l2.Before(testCtx, nil, def2, []uint64{}, nil)
	l2.After(testCtx, nil, def2, []uint64{})
	l1.After(testCtx, nil, def1, []uint64{})
	require.Equal(t, `--> test.fn1()
	--> test.fn2()
	<--
<--
`, out.String())
}

func BenchmarkLoggingListener(b *testing.B) {
	module := wazerotest.NewModule(
		wazerotest.NewMemory(0),
		wazerotest.NewFunction(func(ctx context.Context, mod api.Module) {}),
	)

	function := module.Function(0)
	factory := logging.NewLoggingListenerFactory(discard{})
	listener := factory.NewFunctionListener(function.Definition())

	stack := []experimental.StackFrame{
		{Function: function},
	}

	experimental.BenchmarkFunctionListener(b.N, module, stack, listener)
}

type discard struct{}

func (discard) Write(b []byte) (int, error)       { return len(b), nil }
func (discard) WriteString(s string) (int, error) { return len(s), nil }
