package wasm

import (
	"context"
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestGetFunctionType(t *testing.T) {
	i32, i64, f32, f64 := ValueTypeI32, ValueTypeI64, ValueTypeF32, ValueTypeF64

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
