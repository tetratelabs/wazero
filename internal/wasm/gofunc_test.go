package wasm

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestGetFunctionType(t *testing.T) {
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
			expectedType: &FunctionType{Params: []ValueType{}, Results: []ValueType{}},
		},
		{
			name:         "wasm.Module void return",
			inputFunc:    func(api.Module) {},
			expectedKind: FunctionKindGoModule,
			expectedType: &FunctionType{Params: []ValueType{}, Results: []ValueType{}},
		},
		{
			name:         "context.Context void return",
			inputFunc:    func(context.Context) {},
			expectedKind: FunctionKindGoContext,
			expectedType: &FunctionType{Params: []ValueType{}, Results: []ValueType{}},
		},
		{
			name:         "all supported params and i32 result",
			inputFunc:    func(uint32, uint64, float32, float64) uint32 { return 0 },
			expectedKind: FunctionKindGoNoContext,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64}, Results: []ValueType{i32}},
		},
		{
			name:         "all supported params and all supported results",
			inputFunc:    func(uint32, uint64, float32, float64) (uint32, uint64, float32, float64) { return 0, 0, 0, 0 },
			expectedKind: FunctionKindGoNoContext,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64}, Results: []ValueType{i32, i64, f32, f64}},
		},
		{
			name:         "all supported params and i32 result - wasm.Module",
			inputFunc:    func(api.Module, uint32, uint64, float32, float64) uint32 { return 0 },
			expectedKind: FunctionKindGoModule,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64}, Results: []ValueType{i32}},
		},
		{
			name:         "all supported params and i32 result - context.Context",
			inputFunc:    func(context.Context, uint32, uint64, float32, float64) uint32 { return 0 },
			expectedKind: FunctionKindGoContext,
			expectedType: &FunctionType{Params: []ValueType{i32, i64, f32, f64}, Results: []ValueType{i32}},
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			rVal := reflect.ValueOf(tc.inputFunc)
			fk, ft, err := getFunctionType(&rVal, Features20191205|FeatureMultiValue)
			require.NoError(t, err)
			require.Equal(t, tc.expectedKind, fk)
			require.Equal(t, tc.expectedType, ft)
		})
	}
}

func TestGetFunctionTypeErrors(t *testing.T) {
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
			name:        "multiple results - multi-value not enabled",
			input:       func() (uint64, uint32) { return 0, 0 },
			expectedErr: "multiple result types invalid as feature \"multi-value\" is disabled",
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
			rVal := reflect.ValueOf(tc.input)
			_, _, err := getFunctionType(&rVal, Features20191205)
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
			name:      "all supported params",
			inputFunc: func(uint32, uint64, float32, float64) {},
			expected:  []uint64{4, 5, 6, 7},
		},
		{
			name:      "all supported params - wasm.Module",
			inputFunc: func(api.Module, uint32, uint64, float32, float64) {},
			expected:  []uint64{4, 5, 6, 7},
		},
		{
			name:      "all supported params - context.Context",
			inputFunc: func(context.Context, uint32, uint64, float32, float64) {},
			expected:  []uint64{4, 5, 6, 7},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			goFunc := reflect.ValueOf(tc.inputFunc)
			fk, _, err := getFunctionType(&goFunc, FeaturesFinished)
			require.NoError(t, err)

			vals := PopGoFuncParams(&FunctionInstance{Kind: fk, GoFunc: &goFunc}, (&stack{stackVals}).pop)
			require.Equal(t, tc.expected, vals)
		})
	}
}

func TestCallGoFunc(t *testing.T) {
	expectedCtx, cancel := context.WithCancel(context.Background()) // arbitrary non-default context
	defer cancel()
	callCtx := &CallContext{ctx: expectedCtx}

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
				require.Equal(t, expectedCtx, ctx)
			},
		},
		{
			name: "all supported params and i32 result",
			inputFunc: func(w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
		{
			name: "all supported params and all supported results",
			inputFunc: func(w uint32, x uint64, y float32, z float64) (uint32, uint64, float32, float64) {
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100, 200, 300, 400
			},
			inputParams: []uint64{
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{
				api.EncodeI32(100),
				200,
				api.EncodeF32(300),
				api.EncodeF64(400),
			},
		},
		{
			name: "all supported params and i32 result - wasm.Module",
			inputFunc: func(m api.Module, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, callCtx, m)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
				math.MaxUint32,
				math.MaxUint64,
				api.EncodeF32(math.MaxFloat32),
				api.EncodeF64(math.MaxFloat64),
			},
			expectedResults: []uint64{100},
		},
		{
			name: "all supported params and i32 result - context.Context",
			inputFunc: func(ctx context.Context, w uint32, x uint64, y float32, z float64) uint32 {
				require.Equal(t, expectedCtx, ctx)
				require.Equal(t, uint32(math.MaxUint32), w)
				require.Equal(t, uint64(math.MaxUint64), x)
				require.Equal(t, float32(math.MaxFloat32), y)
				require.Equal(t, math.MaxFloat64, z)
				return 100
			},
			inputParams: []uint64{
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
			goFunc := reflect.ValueOf(tc.inputFunc)
			fk, _, err := getFunctionType(&goFunc, FeaturesFinished)
			require.NoError(t, err)

			results := CallGoFunc(callCtx, &FunctionInstance{Kind: fk, GoFunc: &goFunc}, tc.inputParams)
			require.Equal(t, tc.expectedResults, results)
		})
	}
}
