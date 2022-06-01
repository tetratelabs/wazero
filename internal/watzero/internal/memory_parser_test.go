package internal

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestMemoryParser(t *testing.T) {
	zero := uint32(0)
	max := wasm.MemoryLimitPages
	tests := []struct {
		name       string
		input      string
		expected   *wasm.Memory
		expectedID string
	}{
		{
			name:     "min 0",
			input:    "(memory 0)",
			expected: &wasm.Memory{Max: max},
		},
		{
			name:     "min 0, max 0",
			input:    "(memory 0 0)",
			expected: &wasm.Memory{Max: zero, IsMaxEncoded: true},
		},
		{
			name:     "min largest",
			input:    "(memory 65536)",
			expected: &wasm.Memory{Min: max, Cap: max, Max: max},
		},
		{
			name:       "min largest ID",
			input:      "(memory $mem 65536)",
			expected:   &wasm.Memory{Min: max, Cap: max, Max: max},
			expectedID: "mem",
		},
		{
			name:     "min 0, max largest",
			input:    "(memory 0 65536)",
			expected: &wasm.Memory{Max: max, IsMaxEncoded: true},
		},
		{
			name:     "min largest max largest",
			input:    "(memory 65536 65536)",
			expected: &wasm.Memory{Min: max, Cap: max, Max: max, IsMaxEncoded: true},
		},
		{
			name:       "min largest max largest ID",
			input:      "(memory $mem 65536 65536)",
			expected:   &wasm.Memory{Min: max, Cap: max, Max: max, IsMaxEncoded: true},
			expectedID: "mem",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memoryNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
				require.Equal(t, wasm.SectionIDMemory, sectionID)
				return 0
			})
			parsed, tp, err := parseMemoryType(memoryNamespace, tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, parsed)
			require.Equal(t, uint32(1), tp.memoryNamespace.count)
			if tc.expectedID == "" {
				require.Zero(t, len(tp.memoryNamespace.idToIdx), "expected no indices")
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
			expectedErr: "min 1 pages (64 Ki) > max 0 pages (0 Ki)",
		},
		{
			name:        "min > limit",
			input:       "(memory 4294967295)",
			expectedErr: "min 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
		{
			name:        "max > limit",
			input:       "(memory 0 4294967295)",
			expectedErr: "max 4294967295 pages (3 Ti) over limit of 65536 pages (4 Gi)",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memoryNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
				require.Equal(t, wasm.SectionIDMemory, sectionID)
				return 0
			})
			parsed, _, err := parseMemoryType(memoryNamespace, tc.input)
			require.EqualError(t, err, tc.expectedErr)
			require.Nil(t, parsed)
		})
	}

	t.Run("duplicate ID", func(t *testing.T) {
		memoryNamespace := newIndexNamespace(func(sectionID wasm.SectionID) uint32 {
			require.Equal(t, wasm.SectionIDMemory, sectionID)
			return 0
		})
		_, err := memoryNamespace.setID([]byte("$mem"))
		require.NoError(t, err)
		memoryNamespace.count++

		parsed, _, err := parseMemoryType(memoryNamespace, "(memory $mem 1024)")
		require.EqualError(t, err, "duplicate ID $mem")
		require.Nil(t, parsed)
	})
}

func parseMemoryType(memoryNamespace *indexNamespace, input string) (*wasm.Memory, *memoryParser, error) {
	var parsed *wasm.Memory
	var setFunc onMemory = func(mem *wasm.Memory) tokenParser {
		parsed = mem
		return parseErr
	}
	tp := newMemoryParser(wasm.MemorySizer, memoryNamespace, setFunc)
	// memoryParser starts after the '(memory', so we need to eat it first!
	_, _, err := lex(skipTokens(2, tp.begin), []byte(input))
	return parsed, tp, err
}
