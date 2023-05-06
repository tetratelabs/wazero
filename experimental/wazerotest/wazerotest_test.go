package wazerotest

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/api"
)

func TestNewFunction(t *testing.T) {
	tests := []struct {
		in  any
		out *Function
	}{
		{
			in: func(ctx context.Context, mod api.Module) {},
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v uint32) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeI32},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v uint64) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeI64},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v int32) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeI32},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v int64) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeI64},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v float32) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeF32},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, v float64) {},
			out: &Function{
				ParamTypes:  []api.ValueType{api.ValueTypeF64},
				ResultTypes: []api.ValueType{},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) uint32 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeI32},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) uint64 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeI64},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) int32 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeI32},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) int64 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeI64},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) float32 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeF32},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module) float64 { return 0 },
			out: &Function{
				ParamTypes:  []api.ValueType{},
				ResultTypes: []api.ValueType{api.ValueTypeF64},
			},
		},

		{
			in: func(ctx context.Context, mod api.Module, _ uint32, _ uint64, _ int32, _ int64, _ float32, _ float64) (_ uint32, _ uint64, _ int32, _ int64, _ float32, _ float64) {
				return
			},
			out: &Function{
				ParamTypes: []api.ValueType{
					api.ValueTypeI32, api.ValueTypeI64,
					api.ValueTypeI32, api.ValueTypeI64,
					api.ValueTypeF32, api.ValueTypeF64,
				},
				ResultTypes: []api.ValueType{
					api.ValueTypeI32, api.ValueTypeI64,
					api.ValueTypeI32, api.ValueTypeI64,
					api.ValueTypeF32, api.ValueTypeF64,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(reflect.TypeOf(test.in).String(), func(t *testing.T) {
			f := NewFunction(test.in)
			if !bytes.Equal(test.out.ParamTypes, f.ParamTypes) {
				t.Errorf("invalid parameter types: want=%v got=%v", test.out.ParamTypes, f.ParamTypes)
			}
			if !bytes.Equal(test.out.ResultTypes, f.ResultTypes) {
				t.Errorf("invalid result types: want=%v got=%v", test.out.ResultTypes, f.ResultTypes)
			}
		})
	}
}

func TestNewMemory(t *testing.T) {
	if memory := NewMemory(1); len(memory.Bytes) != PageSize {
		t.Error("memory buffer not aligned on page size")
	}
}

func TestNewFixedMemory(t *testing.T) {
	if memory := NewFixedMemory(1); len(memory.Bytes) != PageSize {
		t.Error("memory buffer not aligned on page size")
	} else if memory.Min != 1 {
		t.Error("invalid min memory size:", memory.Min)
	} else if memory.Max != 1 {
		t.Error("invalid max memory size:", memory.Max)
	}
}
