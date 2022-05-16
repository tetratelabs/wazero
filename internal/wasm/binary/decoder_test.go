package binary

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// TestDecodeModule relies on unit tests for Module.Encode, specifically that the encoding is both known and correct.
// This avoids having to copy/paste or share variables to assert against byte arrays.
func TestDecodeModule(t *testing.T) {
	i32, f32 := wasm.ValueTypeI32, wasm.ValueTypeF32
	zero := uint32(0)

	tests := []struct {
		name  string
		input *wasm.Module // round trip test!
	}{
		{
			name:  "empty",
			input: &wasm.Module{},
		},
		{
			name:  "only name section",
			input: &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}},
		},
		{
			name: "type section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{},
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32},
						ParamNumInUint64:  2,
						ResultNumInUint64: 1,
					},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32},
						ParamNumInUint64:  4,
						ResultNumInUint64: 1,
					},
				},
			},
		},
		{
			name: "type and import section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32},
						ParamNumInUint64:  2,
						ResultNumInUint64: 1,
					},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32},
						ParamNumInUint64:  2,
						ResultNumInUint64: 1,
					},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "Math", Name: "Mul",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Type:     wasm.ExternTypeFunc,
						DescFunc: 0,
					},
				},
			},
		},
		{
			name: "table and memory section",
			input: &wasm.Module{
				TableSection:  []*wasm.Table{{Min: 3, Type: wasm.RefTypeFuncref}},
				MemorySection: &wasm.Memory{Min: 1, Cap: 1, Max: 1, IsMaxEncoded: true},
			},
		},
		{
			name: "type function and start section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Type:     wasm.ExternTypeFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(EncodeModule(tc.input), wasm.Features20191205, wasm.MemorySizer)
			require.NoError(t, e)
			require.Equal(t, tc.input, m)
		})
	}

	t.Run("skips custom section", func(t *testing.T) {
		input := append(append(Magic, version...),
			wasm.SectionIDCustom, 0xf, // 15 bytes in this section
			0x04, 'm', 'e', 'm', 'e',
			1, 2, 3, 4, 5, 6, 7, 8, 9, 0)
		m, e := DecodeModule(input, wasm.Features20191205, wasm.MemorySizer)
		require.NoError(t, e)
		require.Equal(t, &wasm.Module{}, m)
	})

	t.Run("skips custom section, but not name", func(t *testing.T) {
		input := append(append(Magic, version...),
			wasm.SectionIDCustom, 0xf, // 15 bytes in this section
			0x04, 'm', 'e', 'm', 'e',
			1, 2, 3, 4, 5, 6, 7, 8, 9, 0,
			wasm.SectionIDCustom, 0x0e, // 14 bytes in this section
			0x04, 'n', 'a', 'm', 'e',
			subsectionIDModuleName, 0x07, // 7 bytes in this subsection
			0x06, // the Module name simple is 6 bytes long
			's', 'i', 'm', 'p', 'l', 'e')
		m, e := DecodeModule(input, wasm.Features20191205, wasm.MemorySizer)
		require.NoError(t, e)
		require.Equal(t, &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}}, m)
	})
	t.Run("data count section disabled", func(t *testing.T) {
		input := append(append(Magic, version...),
			wasm.SectionIDDataCount, 1, 0)
		_, e := DecodeModule(input, wasm.Features20191205, wasm.MemorySizer)
		require.EqualError(t, e, `data count section not supported as feature "bulk-memory-operations" is disabled`)
	})
}

func TestDecodeModule_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectedErr string
	}{
		{
			name:        "wrong magic",
			input:       []byte("wasm\x01\x00\x00\x00"),
			expectedErr: "invalid magic number",
		},
		{
			name:        "wrong version",
			input:       []byte("\x00asm\x01\x00\x00\x01"),
			expectedErr: "invalid version header",
		},
		{
			name: "multiple start sections",
			input: append(append(Magic, version...),
				wasm.SectionIDType, 4, 1, 0x60, 0, 0,
				wasm.SectionIDFunction, 2, 1, 0,
				wasm.SectionIDCode, 4, 1,
				2, 0, wasm.OpcodeEnd,
				wasm.SectionIDStart, 1, 0,
				wasm.SectionIDStart, 1, 0,
			),
			expectedErr: `multiple start sections are invalid`,
		},
		{
			name: "redundant name section",
			input: append(append(Magic, version...),
				wasm.SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x02, 0x01, 'x',
				wasm.SectionIDCustom, 0x09, // 9 bytes in this section
				0x04, 'n', 'a', 'm', 'e',
				subsectionIDModuleName, 0x02, 0x01, 'x'),
			expectedErr: "section custom: redundant custom section name",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, e := DecodeModule(tc.input, wasm.Features20191205, wasm.MemorySizer)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
