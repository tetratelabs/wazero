package internal

import (
	"errors"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestIndexNamespace_SetId(t *testing.T) {
	in := newIndexNamespace(func(_ wasm.SectionID) uint32 {
		t.Fail()
		return 0
	})
	t.Run("set when empty", func(t *testing.T) {
		id, err := in.setID([]byte("$x"))
		require.NoError(t, err)
		require.Equal(t, "x", id) // strips "$" to be like the name section
		require.Equal(t, map[string]wasm.Index{"x": wasm.Index(0)}, in.idToIdx)
	})
	t.Run("set when exists fails", func(t *testing.T) {
		_, err := in.setID([]byte("$x"))

		// error reflects the original '$' prefix
		require.EqualError(t, err, "duplicate ID $x")
		require.Equal(t, map[string]wasm.Index{"x": wasm.Index(0)}, in.idToIdx) // no change
	})
}

func TestIndexNamespace_Resolve(t *testing.T) {
	for _, tt := range []struct {
		name        string
		namespace   *indexNamespace
		id          string
		numeric     wasm.Index
		expected    wasm.Index
		expectedErr string
	}{
		{
			name:      "numeric exists",
			namespace: &indexNamespace{count: 2},
			numeric:   1,
			expected:  1,
		},
		{
			name:        "numeric, but empty",
			namespace:   &indexNamespace{count: 0},
			numeric:     0,
			expectedErr: "3:4: index 0 is not in range due to empty namespace",
		},
		{
			name:        "numeric out of range",
			namespace:   &indexNamespace{count: 2},
			numeric:     2,
			expectedErr: "3:4: index 2 is out of range [0..1]",
		},
		{
			name:      "ID exists",
			namespace: &indexNamespace{idToIdx: map[string]wasm.Index{"x": 1}},
			id:        "x",
			expected:  1,
		},
		{
			name:        "ID, but empty",
			namespace:   &indexNamespace{idToIdx: map[string]wasm.Index{}},
			id:          "X",
			expectedErr: "3:4: unknown ID $X",
		},
		{
			name:        "ID doesn't exist",
			namespace:   &indexNamespace{idToIdx: map[string]wasm.Index{"x": 1}},
			id:          "X",
			expectedErr: "3:4: unknown ID $X",
		},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			ui := &unresolvedIndex{section: wasm.SectionIDFunction, idx: 1, targetID: tc.id, targetIdx: tc.numeric, line: 3, col: 4}
			index, err := tc.namespace.resolve(ui)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.expected, index)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestIndexNamespace_RequireIndex(t *testing.T) {
	for _, tt := range []struct {
		name        string
		namespace   *indexNamespace
		id          string
		expected    wasm.Index
		expectedErr string
	}{
		{name: "exists", namespace: &indexNamespace{idToIdx: map[string]wasm.Index{"x": 1}}, id: "x", expected: 1},
		{name: "empty", namespace: &indexNamespace{idToIdx: map[string]wasm.Index{}}, id: "X", expectedErr: "unknown ID $X"},
		{name: "doesn't exist", namespace: &indexNamespace{idToIdx: map[string]wasm.Index{"x": 1}}, id: "X", expectedErr: "unknown ID $X"},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			index, err := tc.namespace.requireIndex(tc.id)
			if tc.expectedErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.expected, index)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestUnresolvedIndex_FormatError(t *testing.T) {
	for _, tt := range []struct {
		section     wasm.SectionID
		expectedErr string
	}{
		{section: wasm.SectionIDCode, expectedErr: "3:4: bomb in module.code[1].body[2]"},
		{section: wasm.SectionIDExport, expectedErr: "3:4: bomb in module.exports[1].func"},
		{section: wasm.SectionIDStart, expectedErr: "3:4: bomb in module.start"},
	} {
		tc := tt
		t.Run(wasm.SectionIDName(tc.section), func(t *testing.T) {
			ui := &unresolvedIndex{section: tc.section, idx: 1, bodyOffset: 2, targetID: "X", targetIdx: 2, line: 3, col: 4}
			require.EqualError(t, ui.formatErr(errors.New("bomb")), tc.expectedErr)
		})
	}
}

func TestRequireIndexInRange(t *testing.T) {
	for _, tt := range []struct {
		name         string
		index, count uint32
		expectedErr  string
	}{
		{name: "in range: index 0", index: 0, count: 1},
		{name: "in range: index positive", index: 0, count: 1},
		{name: "out of range: index 0", index: 0, count: 0, expectedErr: "index 0 is not in range due to empty namespace"},
		{name: "out of range: index equals count", index: 3, count: 3, expectedErr: "index 3 is out of range [0..2]"},
		{name: "out of range: index greater than count", index: 4, count: 3, expectedErr: "index 4 is out of range [0..2]"},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			err := requireIndexInRange(tc.index, tc.count)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.Nil(t, err)
			}
		})
	}
}
