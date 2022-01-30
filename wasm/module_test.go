package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionType_String(t *testing.T) {
	tp := FunctionType{}
	require.Equal(t, "null_null", tp.String())

	// With params.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}}
	require.Equal(t, "i32_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i32f64_null", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "f32i32f64_null", tp.String())

	// With results.
	tp = FunctionType{Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "null_i64", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "null_i64f32", tp.String())
	tp = FunctionType{Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "null_f32i32f64", tp.String())

	// With params and results.
	tp = FunctionType{Params: []ValueType{ValueTypeI32}, Results: []ValueType{ValueTypeI64}}
	require.Equal(t, "i32_i64", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32}, Results: []ValueType{ValueTypeI64, ValueTypeF32}}
	require.Equal(t, "i64f32_i64f32", tp.String())
	tp = FunctionType{Params: []ValueType{ValueTypeI64, ValueTypeF32, ValueTypeF64}, Results: []ValueType{ValueTypeF32, ValueTypeI32, ValueTypeF64}}
	require.Equal(t, "i64f32f64_f32i32f64", tp.String())
}

func TestSectionIDName(t *testing.T) {
	tests := []struct {
		name     string
		input    SectionID
		expected string
	}{
		{"custom", SectionIDCustom, "custom"},
		{"type", SectionIDType, "type"},
		{"import", SectionIDImport, "import"},
		{"function", SectionIDFunction, "function"},
		{"table", SectionIDTable, "table"},
		{"memory", SectionIDMemory, "memory"},
		{"global", SectionIDGlobal, "global"},
		{"export", SectionIDExport, "export"},
		{"start", SectionIDStart, "start"},
		{"element", SectionIDElement, "element"},
		{"code", SectionIDCode, "code"},
		{"data", SectionIDData, "data"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, SectionIDName(tc.input))
		})
	}
}

func TestExportKindName(t *testing.T) {
	tests := []struct {
		name     string
		input    ExportKind
		expected string
	}{
		{"func", ExportKindFunc, "func"},
		{"table", ExportKindTable, "table"},
		{"mem", ExportKindMemory, "mem"},
		{"global", ExportKindGlobal, "global"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, exportKindName(tc.input))
		})
	}
}

func TestValueTypeName(t *testing.T) {
	tests := []struct {
		name     string
		input    ValueType
		expected string
	}{
		{"i32", ValueTypeI32, "i32"},
		{"i64", ValueTypeI64, "i64"},
		{"f32", ValueTypeF32, "f32"},
		{"f64", ValueTypeF64, "f64"},
		{"unknown", 100, "unknown"},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, valueTypeName(tc.input))
		})
	}
}
