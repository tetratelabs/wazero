package wasm

import (
	"context"
	"math"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func Test_parseGoFunc(t *testing.T) {
	var tests = []struct {
		name         string
		inputFunc    interface{}
		expectedKind FunctionKind
		expectedType *FunctionType
	}{
		{
			name:         "nullary",
			inputFunc:    func() {},
			expectedKind: FunctionKindGoNoContext,
			expectedType: &FunctionType{},
		},
		{
			name:         "wasm.Module void return",
			inputFunc:    func(api.Module) {},
			expectedKind: FunctionKindGoModule,
			expectedType: &FunctionType{},
		},
		{
			name:         "context.Context void return",
			inputFunc:    func(context.Context) {},
			expectedKind: FunctionKindGoContext,
			expectedType: &FunctionType{},
		},
		{
			name:         "context.Context and api.Module void return",
			inputFunc:    func(context.Context, api.Module) {},
			expectedKind: FunctionKindGoContextModule,
			expectedType: &FunctionType{},
		},
		{
			name:         "all supported params and i32 result",
			inputFunc:    func(uint32, uint64, float32, float64, uintptr) uint32 { return 0 },
			expectedKind: FunctionKindGoNoContext,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64, externref}, Results: []ValueType{i32}},
		},
		{
			name: "all supported params and all supported results",
			inputFunc: func(uint32, uint64, float32, float64, uintptr) (uint32, uint64, float32, float64, uintptr) {
				return 0, 0, 0, 0, 0
			},
			expectedKind: FunctionKindGoNoContext,
			expectedType: &FunctionType{
				Params:  []ValueType{i32, i64, f32, f64, externref},
				Results: []ValueType{i32, i64, f32, f64, externref},
			},
		},
		{
			name:         "all supported params and i32 result - wasm.Module",
			inputFunc:    func(api.Module, uint32, uint64, float32, float64, uintptr) uint32 { return 0 },
			expectedKind: FunctionKindGoModule,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64, externref}, Results: []ValueType{i32}},
		},
		{
			name:         "all supported params and i32 result - context.Context",
			inputFunc:    func(context.Context, uint32, uint64, float32, float64, uintptr) uint32 { return 0 },
			expectedKind: FunctionKindGoContext,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64, externref}, Results: []ValueType{i32}},
		},
		{
			name:         "all supported params and i32 result - context.Context and api.Module",
			inputFunc:    func(context.Context, api.Module, uint32, uint64, float32, float64, uintptr) uint32 { return 0 },
			expectedKind: FunctionKindGoContextModule,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64, externref}, Results: []ValueType{i32}},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			paramTypes, resultTypes, code, err := parseGoFunc(tc.inputFunc)
			require.NoError(t, err)
			require.Equal(t, tc.expectedKind, code.Kind)
			require.Equal(t, tc.expectedType, &FunctionType{Params: paramTypes, Results: resultTypes})
		})
	}
}

func Test_parseGoFunc_Errors(t *testing.T) {
	tests := []struct {
		name             string
		input            interface{}
		allowErrorResult bool
		expectedErr      string
	}{
		{
			name:        "not a func",
			input:       struct{}{},
			expectedErr: "kind != func: struct",
		},
		{
			name:        "unsupported param",
			input:       func(uint32, string) {},
			expectedErr: "param[1] is unsupported: string",
		},
		{
			name:        "unsupported result",
			input:       func() string { return "" },
			expectedErr: "result[0] is unsupported: string",
		},
		{
			name:        "error result",
			input:       func() error { return nil },
			expectedErr: "result[0] is an error, which is unsupported",
		},
		{
			name:        "multiple context types",
			input:       func(api.Module, context.Context) error { return nil },
			expectedErr: "param[1] is a context.Context, which may be defined only once as param[0]",
		},
		{
			name:        "multiple context.Context",
			input:       func(context.Context, uint64, context.Context) error { return nil },
			expectedErr: "param[2] is a context.Context, which may be defined only once as param[0]",
		},
		{
			name:        "multiple wasm.Module",
			input:       func(api.Module, uint64, api.Module) error { return nil },
			expectedErr: "param[2] is a api.Module, which may be defined only once as param[0]",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseGoFunc(tc.input)
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

// stack simulates the value stack in a way easy to be tested.
type stack struct {
	vals []uint64
}

func (s *stack) pop() (result uint64) {
	stackTopIndex := len(s.vals) - 1
	result = s.vals[stackTopIndex]
	s.vals = s.vals[:stackTopIndex]
	return
}

func TestPopValues(t *testing.T) {
	stackVals := []uint64{1, 2, 3, 4, 5, 6, 7}
	var tests = []struct {
		name     string
		count    int
		expected []uint64
	}{
		{
			name: "pop zero doesn't allocate a slice ",
		},
		{
			name:     "pop 1",
			count:    1,
			expected: []uint64{7},
		},
		{
			name:     "pop 2",
			count:    2,
			expected: []uint64{6, 7},
		},
		{
			name:     "pop 3",
			count:    3,
			expected: []uint64{5, 6, 7},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			vals := PopValues(tc.count, (&stack{stackVals}).pop)
			require.Equal(t, tc.expected, vals)
		})
	}
}

func TestPopGoFuncParams(t *testing.T) {
	stackVals := []uint64{1, 2, 3, 4, 5, 6, 7}
	var tests = []struct {
		name      string
		inputFunc interface{}
		expected  []uint64
	}{
		{
			name:      "nullary",
			inputFunc: func() {},
		},
		{
			name:      "wasm.Module",
			inputFunc: func(api.Module) {},
		},
		{
			name:      "context.Context",
			inputFunc: func(context.Context) {},
		},
		{
			name:      "context.Context and api.Module",
			inputFunc: func(context.Context, api.Module) {},
		},
		{
			name:      "all supported params",
			inputFunc: func(uint32, uint64, float32, float64, uintptr) {},
			expected:  []uint64{3, 4, 5, 6, 7},
		},
		{
			name:      "all supported params - wasm.Module",
			inputFunc: func(api.Module, uint32, uint64, float32, float64, uintptr) {},
			expected:  []uint64{3, 4, 5, 6, 7},
		},
		{
			name:      "all supported params - context.Context",
			inputFunc: func(context.Context, uint32, uint64, float32, float64, uintptr) {},
			expected:  []uint64{3, 4, 5, 6, 7},
		},
		{
			name:      "all supported params - context.Context and api.Module",
			inputFunc: func(context.Context, api.Module, uint32, uint64, float32, float64, uintptr) {},
			expected:  []uint64{3, 4, 5, 6, 7},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, _, code, err := parseGoFunc(tc.inputFunc)
			require.NoError(t, err)

			vals := PopGoFuncParams(&FunctionInstance{Kind: code.Kind, GoFunc: code.GoFunc}, (&stack{stackVals}).pop)
			require.Equal(t, tc.expected, vals)
		})
	}
}

func TestCallGoFunc(t *testing.T) {
	tPtr := uintptr(unsafe.Pointer(t))
	callCtx := &CallContext{}
	callCtxPtr := uintptr(unsafe.Pointer(callCtx))

	var tests = []struct {
		name                         string
		inputFunc                    interface{}
		inputParams, expectedResults []uint64
	}{
		{
			name:      "nullary",
			inputFunc: func() {},
		},
		{
			name: "wasm.Module void return",
			inputFunc: func(m api.Module) {
				require.Equal(t, callCtx, m)
			},
		},
		{
			name: "context.Context void return",
			inputFunc: func(ctx context.Context) {
				require.Equal(t, testCtx, ctx)
			},
		},
		{
			name: "context.Context and api.Module void return",
			inputFunc: func(ctx context.Context, m api.Module) {
				require.Equal(t, testCtx, ctx)
				require.Equal(t, callCtx, m)
			},
		},
		{
			name: "all supported params and i32 result",
			inputFunc: func(v uintptr, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, tPtr, v)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				api.EncodeExternref(tPtr),
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
		{
			name: "all supported params and all supported results",
			inputFunc: func(v uintptr, w uint32, x uint64, y float32, z float64) (uintptr, uint32, uint64, float32, float64) {
				require.Equal(t, tPtr, v)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return uintptr(unsafe.Pointer(callCtx)), 100, 200, 300, 400
			},
			inputParams: []uint64{
				api.EncodeExternref(tPtr),
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{
				api.EncodeExternref(callCtxPtr),
				api.EncodeI32(100),
				200,
				api.EncodeF32(300),
				api.EncodeF64(400),
			},
		},
		{
			name: "all supported params and i32 result - wasm.Module",
			inputFunc: func(m api.Module, v uintptr, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, callCtx, m)
				require.Equal(t, tPtr, v)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				api.EncodeExternref(tPtr),
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
		{
			name: "all supported params and i32 result - context.Context",
			inputFunc: func(ctx context.Context, v uintptr, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, testCtx, ctx)
				require.Equal(t, tPtr, v)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				api.EncodeExternref(tPtr),
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
		{
			name: "all supported params and i32 result - context.Context and api.Module",
			inputFunc: func(ctx context.Context, m api.Module, v uintptr, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, testCtx, ctx)
				require.Equal(t, callCtx, m)
				require.Equal(t, tPtr, v)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				api.EncodeExternref(tPtr),
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			paramTypes, resultTypes, code, err := parseGoFunc(tc.inputFunc)
			require.NoError(t, err)

			results := CallGoFunc(
				testCtx,
				callCtx,
				&FunctionInstance{
					IsHostFunction: code.IsHostFunction,
					Kind:           code.Kind,
					Type:           &FunctionType{Params: paramTypes, Results: resultTypes},
					GoFunc:         code.GoFunc,
				},
				tc.inputParams,
			)
			require.Equal(t, tc.expectedResults, results)
		})
	}
}
