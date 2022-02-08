package text

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
)

func TestMemoryParser(t *testing.T) {
	zero := uint32(0)
	max := wasm.MemoryMaxPages
	tests := []struct {
		name       string
		input      string
		expected   *wasm.MemoryType
		expectedID string
	}{
		{
			name:     "min 0",
			input:    "(memory 0)",
			expected: &wasm.MemoryType{},
		},
		{
			name:     "min 0, max 0",
			input:    "(memory 0 0)",
			expected: &wasm.MemoryType{Max: &zero},
		},
		{
			name:     "min largest",
			input:    "(memory 65536)",
			expected: &wasm.MemoryType{Min: max},
		},
		{
			name:       "min largest - ID",
			input:      "(memory $mem 65536)",
			expected:   &wasm.MemoryType{Min: max},
			expectedID: "mem",
		},
		{
			name:     "min 0, max largest",
			input:    "(memory 0 65536)",
			expected: &wasm.MemoryType{Max: &max},
		},
		{
			name:     "min largest max largest",
			input:    "(memory 65536 65536)",
			expected: &wasm.MemoryType{Min: max, Max: &max},
		},
		{
			name:       "min largest max largest - ID",
			input:      "(memory $mem 65536 65536)",
			expected:   &wasm.MemoryType{Min: max, Max: &max},
			expectedID: "mem",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memoryNamespace := newIndexNamespace(wasm.SectionIDMemory)
			parsed, tp, err := parseMemoryType(memoryNamespace, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, parsed)
			if tc.expectedID == "" {
				require.Empty(t, tp.memoryNamespace.idToIdx)
			} else {
				// Since the parser was initially empty, the expected index of the parsed memory is 0
				require.Equal(t, map[string]wasm.Index{tc.expectedID: wasm.Index(0)}, tp.memoryNamespace.idToIdx)
			}
		})
	}
}

func TestMemoryParser_Errors(t *testing.T) {
	tests := []struct{ name, input, expectedErr string }{
		{
			name:        "invalid token",
			input:       "(memory \"0\")",
			expectedErr: "unexpected string: \"0\"",
		},
		{
			name:        "not ID",
			input:       "(memory mem)",
			expectedErr: "unexpected keyword: mem",
		},
		{
			name:        "redundant ID",
			input:       "(memory $mem $0)",
			expectedErr: "redundant ID $0",
		},
		{
			name:        "invalid after ID",
			input:       "(memory $mem frank)",
			expectedErr: "unexpected keyword: frank",
		},
		{
			name:        "missing min",
			input:       "(memory)",
			expectedErr: "missing min",
		},
		{
			name:        "invalid after min",
			input:       "(memory 0 $0)",
			expectedErr: "unexpected ID: $0",
		},
		{
			name:        "invalid after max",
			input:       "(memory 0 0 $0)",
			expectedErr: "unexpected ID: $0",
		},
		{
			name:        "max < min",
			input:       "(memory 1 0)",
			expectedErr: "max 0 < min 1",
		},
		{
			name:        "min > limit",
			input:       "(memory 4294967295)",
			expectedErr: "min outside range of 65536: 4294967295",
		},
		{
			name:        "max > limit",
			input:       "(memory 0 4294967295)",
			expectedErr: "min outside range of 65536: 4294967295",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			parsed, _, err := parseMemoryType(newIndexNamespace(wasm.SectionIDMemory), tc.input)
			require.EqualError(t, err, tc.expectedErr)
			require.Nil(t, parsed)
		})
	}

	t.Run("duplicate ID", func(t *testing.T) {
		memoryNamespace := newIndexNamespace(wasm.SectionIDMemory)
		_, err := memoryNamespace.setID([]byte("$mem"))
		require.NoError(t, err)
		memoryNamespace.count++

		parsed, _, err := parseMemoryType(memoryNamespace, "(memory $mem 1024)")
		require.EqualError(t, err, "duplicate ID $mem")
		require.Nil(t, parsed)
	})
}

func parseMemoryType(memoryNamespace *indexNamespace, input string) (*wasm.MemoryType, *memoryParser, error) {
	var parsed *wasm.MemoryType
	var setFunc onMemory = func(min uint32, max *uint32) tokenParser {
		parsed = &wasm.MemoryType{Min: min, Max: max}
		return parseErr
	}
	tp := newMemoryParser(memoryNamespace, setFunc)
	// memoryParser starts after the '(memory', so we need to eat it first!
	_, _, err := lex(skipTokens(2, tp.begin), []byte(input))
	return parsed, tp, err
}
