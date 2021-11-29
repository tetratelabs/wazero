package wasm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeModule(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expected    *Module
		expectedErr string
	}{
		{
			name:     "empty",
			input:    []byte("\x00asm\x01\x00\x00\x00"),
			expected: &Module{CustomSections: map[string][]byte{}},
		},
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
	}

	for _, tc := range tests {
		tc := tc // pin! see https://github.com/kyoh86/scopelint for why

		t.Run(tc.name, func(t *testing.T) {
			m, e := DecodeModule(tc.input)
			if tc.expectedErr != "" {
				require.EqualError(t, e, tc.expectedErr)
			} else {
				require.NoError(t, e)
				require.Equal(t, tc.expected, m)
			}
		})
	}
}

func TestModuleGetFunctionNames(t *testing.T) {
	m := Module{
		CustomSections: map[string][]byte{},
	}
	// Name section not found.
	_, err := m.GetFunctionNames()
	require.Error(t, err)

	// Name section found, but cannot read subsection ID
	m.CustomSections["name"] = []byte{}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read subsection")

	// Cannot read the subsection size
	m.CustomSections["name"] = []byte{0x04}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read the size of subsection 4")

	// Function name subsection found, but name vector size not found.
	m.CustomSections["name"] = []byte{0x01, 0x00}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read the size of name vector: EOF")

	// Function name subsection found with name vector size=1.
	// But cannot read the vector content with EOF.
	m.CustomSections["name"] = []byte{0x01, 0x01, 0x01}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read function index: EOF")
	m.CustomSections["name"] = []byte{0x01, 0x01, 0x01, 0x00}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read function name size: EOF")
	m.CustomSections["name"] = []byte{0x01, 0x01, 0x01, 0x00, 0x01}
	_, err = m.GetFunctionNames()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read function name: EOF")

	// Valid inputs.
	m.CustomSections["name"] = []byte{
		0x01 /* function subsection id */, 0x04, /* subsection size*/
		0x01, /* size of name map */
		0x00 /* function index*/, 0x01 /* size of name */, 'a',
	}
	names, err := m.GetFunctionNames()
	require.NoError(t, err)
	require.Equal(t, "a", names[0])

	m.CustomSections["name"] = []byte{
		0x00, 0x00, // other subsections.
		0x03, 0x01, 0x00, // other subsections.
		0x01 /* function subsection id */, 0x04, /* subsection size*/
		0x02, /* size of name map */
		0x00 /* function index*/, 0x01 /* size of name */, 'a',
		0x01 /* function index*/, 0x02 /* size of name */, 'a', 'b',
	}
	names, err = m.GetFunctionNames()
	require.NoError(t, err)
	require.Equal(t, "a", names[0])
	require.Equal(t, "ab", names[1])
}
