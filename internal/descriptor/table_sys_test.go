package descriptor_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestFileTable(t *testing.T) {
	table := new(sys.FileTable)

	if n := table.Len(); n != 0 {
		t.Errorf("new table is not empty: length=%d", n)
	}

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
		if v, ok := table.Lookup(lookup.key); !ok {
			t.Errorf("value not found for key '%v'", lookup.key)
		} else if v.Name != lookup.val.Name {
			t.Errorf("wrong value returned for key '%v': want=%v got=%v", lookup.key, lookup.val.Name, v.Name)
		}
	}

	if n := table.Len(); n != 3 {
		t.Errorf("wrong table length: want=3 got=%d", n)
	}

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
		if v.Name != want.Name {
			t.Errorf("wrong value found ranging over '%v': want=%v got=%v", k, want.Name, v.Name)
		}
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
		if !found.ok {
			t.Errorf("key not found while ranging over table: %v", found.key)
		}
	}

	for i, deletion := range []struct {
		key int32
	}{
		{key: k1},
		{key: k0},
		{key: k2},
	} {
		table.Delete(deletion.key)
		if _, ok := table.Lookup(deletion.key); ok {
			t.Errorf("item found after deletion of '%v'", deletion.key)
		}
		if n, want := table.Len(), 3-(i+1); n != want {
			t.Errorf("wrong table length after deletion: want=%d got=%d", want, n)
		}
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
