package descriptor_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/descriptor"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFileTable(t *testing.T) {
	table := new(sys.FileTable)

	n := table.Len()
	require.Equal(t, 0, n, "new table is not empty: length=%d", n)

	// The id field is used as a sentinel value.
	v0 := &sys.FileEntry{Name: "1"}
	v1 := &sys.FileEntry{Name: "2"}
	v2 := &sys.FileEntry{Name: "3"}

	k0, ok := table.Insert(v0)
	require.True(t, ok)
	k1, ok := table.Insert(v1)
	require.True(t, ok)
	k2, ok := table.Insert(v2)
	require.True(t, ok)

	// Try to re-order, but to an invalid value
	ok = table.InsertAt(v2, -1)
	require.False(t, ok)

	for _, lookup := range []struct {
		key int32
		val *sys.FileEntry
	}{
		{key: k0, val: v0},
		{key: k1, val: v1},
		{key: k2, val: v2},
	} {
		v, ok := table.Lookup(lookup.key)
		require.True(t, ok, "value not found for key '%v'", lookup.key)
		require.Equal(t, lookup.val.Name, v.Name, "wrong value returned for key '%v'", lookup.key)
	}

	require.Equal(t, 3, table.Len(), "wrong table length: want=3 got=%d", table.Len())

	k0Found := false
	k1Found := false
	k2Found := false
	table.Range(func(k int32, v *sys.FileEntry) bool {
		var want *sys.FileEntry
		switch k {
		case k0:
			k0Found, want = true, v0
		case k1:
			k1Found, want = true, v1
		case k2:
			k2Found, want = true, v2
		}
		require.Equal(t, want.Name, v.Name, "wrong value found ranging over table")
		return true
	})

	for _, found := range []struct {
		key int32
		ok  bool
	}{
		{key: k0, ok: k0Found},
		{key: k1, ok: k1Found},
		{key: k2, ok: k2Found},
	} {
		require.True(t, found.ok, "key not found while ranging over table: %v", found.key)
	}

	for i, deletion := range []struct {
		key int32
	}{
		{key: k1},
		{key: k0},
		{key: k2},
	} {
		table.Delete(deletion.key)
		_, ok := table.Lookup(deletion.key)
		require.False(t, ok, "item found after deletion of '%v'", deletion.key)
		n, want := table.Len(), 3-(i+1)
		require.Equal(t, want, n, "wrong table length after deletion: want=%d got=%d", want, n)
	}
}

func BenchmarkFileTableInsert(b *testing.B) {
	table := new(sys.FileTable)
	entry := new(sys.FileEntry)

	for i := 0; i < b.N; i++ {
		table.Insert(entry)

		if (i % 65536) == 0 {
			table.Reset() // to avoid running out of memory
		}
	}
}

func BenchmarkFileTableLookup(b *testing.B) {
	const sentinel = "42"
	const numFiles = 65536
	table := new(sys.FileTable)
	files := make([]int32, numFiles)
	entry := &sys.FileEntry{Name: sentinel}

	var ok bool
	for i := range files {
		files[i], ok = table.Insert(entry)
		if !ok {
			b.Fatal("unexpected failure to insert")
		}
	}

	var f *sys.FileEntry
	for i := 0; i < b.N; i++ {
		f, _ = table.Lookup(files[i%numFiles])
	}
	if f.Name != sentinel {
		b.Error("wrong file returned by lookup")
	}
}

func Test_sizeOfTable(t *testing.T) {
	tests := []struct {
		name         string
		operation    func(*descriptor.Table[int32, string])
		expectedSize int
	}{
		{
			name:         "empty table",
			operation:    func(table *descriptor.Table[int32, string]) {},
			expectedSize: 0,
		},
		{
			name: "1 insert",
			operation: func(table *descriptor.Table[int32, string]) {
				table.Insert("a")
			},
			expectedSize: 1,
		},
		{
			name: "32 inserts",
			operation: func(table *descriptor.Table[int32, string]) {
				for i := 0; i < 32; i++ {
					table.Insert("a")
				}
			},
			expectedSize: 1,
		},
		{
			name: "257 inserts",
			operation: func(table *descriptor.Table[int32, string]) {
				for i := 0; i < 257; i++ {
					table.Insert("a")
				}
			},
			expectedSize: 5,
		},
		{
			name: "1 insert at 63",
			operation: func(table *descriptor.Table[int32, string]) {
				table.InsertAt("a", 63)
			},
			expectedSize: 1,
		},
		{
			name: "1 insert at 64",
			operation: func(table *descriptor.Table[int32, string]) {
				table.InsertAt("a", 64)
			},
			expectedSize: 2,
		},
		{
			name: "1 insert at 257",
			operation: func(table *descriptor.Table[int32, string]) {
				table.InsertAt("a", 257)
			},
			expectedSize: 5,
		},
		{
			name: "insert at until 320",
			operation: func(table *descriptor.Table[int32, string]) {
				for i := int32(0); i < 320; i++ {
					table.InsertAt("a", i)
				}
			},
			expectedSize: 5,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			table := new(descriptor.Table[int32, string])
			tc.operation(table)
			require.Equal(t, tc.expectedSize, len(descriptor.Masks(table)))
			require.Equal(t, tc.expectedSize*64, len(descriptor.Items(table)))
		})
	}
}
