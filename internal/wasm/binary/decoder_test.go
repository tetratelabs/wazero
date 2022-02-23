package binary

import (
	"testing"

	"github.com/stretchr/testify/require"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
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
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{i32, i32, i32, i32}, Results: []wasm.ValueType{i32}},
				},
			},
		},
		{
			name: "type and import section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}},
					{Params: []wasm.ValueType{f32, f32}, Results: []wasm.ValueType{f32}},
				},
				ImportSection: []*wasm.Import{
					{
						Module: "Math", Name: "Mul",
						Kind:     wasm.ImportKindFunc,
						DescFunc: 1,
					}, {
						Module: "Math", Name: "Add",
						Kind:     wasm.ImportKindFunc,
						DescFunc: 0,
					},
				},
			},
		},
		{
			name: "memory and export section",
			input: &wasm.Module{
				MemorySection: []*wasm.MemoryType{{Min: 0, Max: &zero}},
				ExportSection: map[string]*wasm.Export{
					"mem": {
						Name:  "mem",
						Kind:  wasm.ExportKindMemory,
						Index: 0,
					},
				},
			},
		},
		{
			name: "type function and start section",
			input: &wasm.Module{
				TypeSection: []*wasm.FunctionType{{}},
				ImportSection: []*wasm.Import{{
					Module: "", Name: "hello",
					Kind:     wasm.ImportKindFunc,
					DescFunc: 0,
				}},
				StartSection: &zero,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(EncodeModule(tc.input))
			require.NoError(t, e)
			require.Equal(t, tc.input, m)
		})
	}
	t.Run("skips custom section", func(t *testing.T) {
		input := append(append(Magic, version...),
			wasm.SectionIDCustom, 0xf, // 15 bytes in this section
			0x04, 'm', 'e', 'm', 'e',
			1, 2, 3, 4, 5, 6, 7, 8, 9, 0)
		m, e := DecodeModule(input)
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
		m, e := DecodeModule(input)
		require.NoError(t, e)
		require.Equal(t, &wasm.Module{NameSection: &wasm.NameSection{ModuleName: "simple"}}, m)
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
			_, e := DecodeModule(tc.input)
			require.EqualError(t, e, tc.expectedErr)
		})
	}
}
